# Server graceful restart with Go

## Install and run the server

```
$ git clone
$ go build
$ ./go-graceful-restart-example
2014/12/14 20:26:42 [Server - 4301] Listen on [::]:12345
[...]
```

## Connect with the client

```
$ cd $GOPATH/src/github.com/Scalingo/go-graceful-restart-example/client
$ go run pong.go
```

## Graceful restart

```
# The server pid is included in its log, in the example: 4301

$ kill -HUP <server pid>
```

## Stop Process


```
$ kill -TERM <server pid>
```
