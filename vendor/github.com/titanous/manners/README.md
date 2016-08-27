# Manners

A *polite* webserver for Go.

Manners allows you to shut your Go webserver down gracefully, without dropping any requests. It can act as a drop-in replacement for the standard library's http.ListenAndServe function:

```go
func main() {
  handler := MyHTTPHandler()
  server := manners.NewServer()
  server.ListenAndServe(":7000", handler)
}
```

Then, when you want to shut the server down:

```go
server.Shutdown <- true
```

(Note that this does not block until all the requests are finished. Rather, the call to server.ListenAndServe will stop blocking when all the requests are finished.)

Manners ensures that all requests are served by incrementing a WaitGroup when a request comes in and decrementing it when the request finishes.

If your request handler spawns Goroutines that are not guaranteed to finish with the request, you can ensure they are also completed with the `StarRoutine` and `FinishRoutine` functions on the server.

### Installation

`go get github.com/braintree/manners`

### Contributors

- [Lionel Barrow](http://github.com/lionelbarrow)
- [Paul Rosenzweig](http://github.com/paulrosenzweig)
