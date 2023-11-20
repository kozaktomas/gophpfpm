# GOPHPFPM

```
Web server for PHP written in Go. It's compatible with PHP-FPM communicating via FastCGI protocol using unix socket.

Usage:
  gophpfpm [flags]

Flags:
      --access-log                  Enable access logging
      --app string                  Application name (default "php-app")
      --fpm-pool-size int           Size of the FPM pool (default 32)
  -h, --help                        help for gofpmproxy
      --index-file string           Path to index.php script in the PHP-FPM container (default "i")
  -p, --port int                    Go FPM proxy port (default 8080)
      --socket string               Path to PHP-FPM UNIX Socket (default "s")
  -f, --static-folder stringArray   Static folder in format "/home/path/to/folder:/endpoint/prefix"
  -v, --verbose                     Print debug output
```

## Features

### Prometheus metrics

The server exposes Prometheus metrics on `/metrics` endpoint. You can set up your own endpoint by setting `X-App-Route`
header in your PHP application (header is not propagated to client).

### Using UNIX socket

The fastest way to communicate with PHP-FPM is to use UNIX socket. You can set up your PHP-FPM process and then pass
socket path to the web server. For **docker-compose** you can use a mount volume to share socket between containers. In
**Kubernetes** you can you EmptyDir volume.

**PHP-FPM config**

```
[global]
daemonize = no
[www]
listen = /sock/php-fpm.sock
listen.mode = 0666
```

### Static files

Server can serve static content. It's recommended to use different approach for serving static files, but if you need,
gophpfmp is ready. You can set up multiple static folders. Each folder is mapped to a different endpoint. For example
`/static` endpoint can be mapped to `/home/app/static` folder. For more info see `--static-folder` flag.

### Security

There is no way how to call other scripts. It's always a PHP file specified in configuration. It's suitable for modern
PHP frameworks like Symfony. No .htaccess, no routing. 
