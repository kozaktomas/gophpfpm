package main

// https://fastcgi-archives.github.io/FastCGI_Specification.html
// http://www.mit.edu/~yandros/doc/specs/fcgi-spec.html

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	FCGI_VERSION = 1

	FCGI_RESPONDER = 1

	FCGI_BEGIN_REQUEST = 1
	FCGI_END_REQUEST   = 3
	FCGI_PARAMS        = 4
	FCGI_STDIN         = 5
	FCGI_STDOUT        = 6
	FCGI_STDERR        = 7
)

type FCgiRecord struct {
	Version       byte
	Type          byte
	RequestId     uint16 // 2 bytes
	ContentLength uint16 // 2 bytes
	PaddingLength byte
	Reserved      byte
}

type FCgiRequest struct {
	Params map[string]string
	Body   []byte

	requestId uint16
}

type FCgiClient struct {
	// slots limits the number of concurrent FPM requests to config.FpmPoolSize
	slots chan struct{}

	config *Config
	logger *log.Logger
}

// FCgiConnection is a single FastCGI connection that carries exactly one request.
type FCgiConnection struct {
	Conn net.Conn
}

// NewFCgiClient creates a FastCGI client. Every request is sent over its own
// connection, which is closed when the request finishes. Reusing connections
// (FCGI_KEEP_CONN) lets unread request body bytes be parsed by PHP-FPM as a
// new, forged FastCGI request (CVE-2026-129289, GHSA-qc49-hpvq-g2w6).
func NewFCgiClient(config *Config, logger *log.Logger) (*FCgiClient, error) {
	if config.FpmPoolSize < 1 {
		return nil, fmt.Errorf("FPM pool size must be at least 1, got %d", config.FpmPoolSize)
	}

	// fail fast when the socket is not reachable at all
	probe, err := net.Dial("unix", config.Socket)
	if err != nil {
		return nil, fmt.Errorf("could not connect to FPM socket: %w", err)
	}
	_ = probe.Close()

	logger.Debugf("FPM client allows %d concurrent requests.", config.FpmPoolSize)

	return &FCgiClient{
		slots: make(chan struct{}, config.FpmPoolSize),

		config: config,
		logger: logger,
	}, nil
}

func (client *FCgiClient) NewRequest(params map[string]string, body []byte) FCgiRequest {
	return FCgiRequest{
		Params: params,
		Body:   body,

		requestId: client.generateRequestId(),
	}
}

// generateRequestId generates unique request id for every request
func (client *FCgiClient) generateRequestId() uint16 {
	token := make([]byte, 2)
	_, _ = rand.Read(token)
	generated := binary.BigEndian.Uint16(token)

	return generated
}

// acquireSlot waits until fewer than FpmPoolSize requests are in flight
func (client *FCgiClient) acquireSlot() {
	for {
		timer := time.After(1 * time.Second)
		select {
		case <-timer:
			client.logger.Infof("It seems that all %d FPM slots are busy", client.config.FpmPoolSize)
		case client.slots <- struct{}{}:
			return
		}
	}
}

// SendRequest sends request to FPM server over a new connection and closes it afterwards
func (client *FCgiClient) SendRequest(r FCgiRequest) (*http.Response, error) {
	client.acquireSlot()
	defer func() {
		<-client.slots
	}()

	netConn, err := net.Dial("unix", client.config.Socket)
	if err != nil {
		return nil, fmt.Errorf("could not connect to FPM socket: %w", err)
	}
	defer func() {
		_ = netConn.Close()
	}()

	if client.config.Timeout > 0 {
		// do not let a stuck FPM worker hold the connection and the slot forever
		_ = netConn.SetDeadline(time.Now().Add(client.config.Timeout))
	}

	conn := &FCgiConnection{Conn: netConn}
	response, err := conn.doRequest(r)
	if err != nil {
		return nil, fmt.Errorf("could not send the request: %w", err)
	}

	return response, nil
}

// Close is a no-op: connections are closed after every request
func (client *FCgiClient) Close() {}

func (c *FCgiConnection) doRequest(r FCgiRequest) (*http.Response, error) {
	var err error
	if err = c.sendHeader(r); err != nil {
		return nil, fmt.Errorf("could not send header: %w", err)
	}
	if err = c.sendParams(r); err != nil {
		return nil, fmt.Errorf("could not send params: %w", err)
	}
	if err = c.sendBody(r); err != nil {
		return nil, fmt.Errorf("could not send body: %w", err)
	}

	resp, err := c.readResponse(r)
	if err != nil {
		return nil, fmt.Errorf("could not read response: %w", err)
	}

	return resp, nil
}

func (c *FCgiConnection) sendHeader(r FCgiRequest) error {
	// no FCGI_KEEP_CONN: FPM closes the connection after the request
	flags := byte(0)
	role := FCGI_RESPONDER
	data := [8]byte{byte(role >> 8), byte(role), flags}
	return c.writeRecord(r.requestId, FCGI_BEGIN_REQUEST, data[:]) // probably delete slicing
}

func (c *FCgiConnection) sendParams(r FCgiRequest) error {
	if len(r.Body) > 0 {
		r.Params["CONTENT_LENGTH"] = strconv.Itoa(len(r.Body))
	}
	for name, value := range r.Params {
		buf := bytes.NewBuffer([]byte{})

		b := make([]byte, 8)
		binary.BigEndian.PutUint32(b, uint32(len(name))|1<<31)
		buf.Write(b[:4])

		binary.BigEndian.PutUint32(b, uint32(len(value))|1<<31)
		buf.Write(b[:4])

		buf.WriteString(name)
		buf.WriteString(value)

		err := c.writeRecord(r.requestId, FCGI_PARAMS, buf.Bytes())
		if err != nil {
			return err
		}
	}

	// end of parameters
	return c.writeRecord(r.requestId, FCGI_PARAMS, []byte{})
}

// contentData: Between 0 and 65535 bytes of data, interpreted according to the record type.
func (c *FCgiConnection) sendBody(r FCgiRequest) error {
	if len(r.Body) > 0 {
		chunkSize := 65535
		for i := 0; i < len(r.Body); i += chunkSize {
			end := i + chunkSize
			if end > len(r.Body) {
				end = len(r.Body)
			}
			if err := c.writeRecord(r.requestId, FCGI_STDIN, r.Body[i:end]); err != nil {
				return err
			}
		}
	}
	return c.writeRecord(r.requestId, FCGI_STDIN, []byte{})
}

func (c *FCgiConnection) readResponse(req FCgiRequest) (*http.Response, error) {
	var stdout []byte

	// read records till we find FCGI_END_REQUEST record
	for {
		respHeader := FCgiRecord{}
		err := binary.Read(c.Conn, binary.BigEndian, &respHeader)
		if err != nil {
			return nil, fmt.Errorf("could not read record header: %w", err)
		}

		if req.requestId != respHeader.RequestId {
			continue
		}

		b := make([]byte, respHeader.ContentLength+uint16(respHeader.PaddingLength))
		err = binary.Read(c.Conn, binary.BigEndian, &b)
		if err != nil {
			return nil, fmt.Errorf("could not read record body: %w", err)
		}

		if respHeader.Type == FCGI_STDOUT {
			stdout = append(stdout, b[:respHeader.ContentLength]...)
		}

		// FCGI_STDERR is read but intentionally discarded

		if respHeader.Type == FCGI_END_REQUEST {
			break
		}
	}

	stdout = append([]byte("HTTP/1.0 200 OK\r\n"), stdout...)

	httpResponse, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(stdout)), nil)
	if err != nil {
		return nil, fmt.Errorf("could not read response as http response: %w", err)
	}

	// parse status
	status := httpResponse.Header.Get("Status")
	if status != "" {
		httpResponse.Status = status
		s := strings.Split(status, " ")
		if len(s) < 2 {
			return nil, fmt.Errorf("could not parse status code: %w", err)
		}

		code, err := strconv.Atoi(s[0])
		if err != nil {
			return nil, fmt.Errorf("could not parse status code: %w", err)
		}
		httpResponse.StatusCode = code
	}

	return httpResponse, nil
}

func (c *FCgiConnection) writeRecord(requestId uint16, recordType byte, contentData []byte) error {
	contentLength := len(contentData)

	// prepare record header
	header := &FCgiRecord{
		Version:       FCGI_VERSION,
		Type:          recordType,
		RequestId:     requestId,
		ContentLength: uint16(contentLength),
		PaddingLength: byte(-contentLength & 7),
	}

	// encode the header
	buf := bytes.NewBuffer([]byte{})
	err := binary.Write(buf, binary.BigEndian, header)
	if err != nil {
		// this should really never happen
		return fmt.Errorf("could not write header: %w", err)
	}

	// write the header to the connection
	_, err = io.Copy(c.Conn, buf)
	if err != nil {
		return fmt.Errorf("could not write header to connection: %w", err)
	}

	// write data to the connection
	_, err = c.Conn.Write(contentData)
	if err != nil {
		return fmt.Errorf("could not write data to connection: %w", err)
	}

	// write padding to the connection
	pad := make([]byte, header.PaddingLength)
	_, err = c.Conn.Write(pad)
	if err != nil {
		return fmt.Errorf("could not write padding to connection: %w", err)
	}

	return nil
}
