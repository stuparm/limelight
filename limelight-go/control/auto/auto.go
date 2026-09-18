// Package auto registers limelight's control routes on http.DefaultServeMux as
// a side effect of being imported, the way net/http/pprof and expvar do:
//
//	import _ "github.com/stuparm/limelight/limelight-go/control/auto"
//
// It exists as its own package rather than as an init() in control/ so that the
// import site says what it is doing. Importing control/ to call Mount or Wrap
// arms nothing; importing this arms the endpoint. That distinction is the
// reason for the package, and it is the one thing net/http/pprof cannot offer —
// there, the only import is the one with the side effect.
//
// # Two things to know before using it
//
// It only works if your process serves http.DefaultServeMux. Most services do
// not: gin, echo, chi, grpc-gateway and any hand-rolled mux all serve their
// own. Against those, this import compiles, runs, registers onto a mux nobody
// serves, and every request 404s with nothing to debug. Use control.Mount or
// control.Wrap instead.
//
// It also leaves no seam for authentication. The routes are on
// http.DefaultServeMux before main runs, so there is nowhere to put middleware
// in front of just them — wrapping DefaultServeMux applies to the whole
// process. Since this endpoint turns on emission of user identifiers, the
// intended shape is a separate admin listener that is not routable from
// outside:
//
//	go func() { log.Fatal(http.ListenAndServe("localhost:6060", nil)) }()
//
// That is what net/http/pprof's own documentation recommends, and why its
// example binds localhost rather than every interface. If instead you need the
// endpoint behind auth on a mux you already serve, use control.Handler:
//
//	mux.Handle(control.Prefix, authMiddleware(control.Handler()))
package auto

import (
	"net/http"

	"github.com/stuparm/limelight/limelight-go/control"
)

func init() {
	control.Mount(http.DefaultServeMux)
}
