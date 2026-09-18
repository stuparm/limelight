# Go example

A service with one tagged method, the control endpoint mounted, and two requests a
second going through it — one for project `abc-123`, one for `zzz-999`.

## Run it

The directive only does something when the build goes through the shim:

```
go build -o /tmp/limelight ../../limelight-go/cmd/limelight
go run -toolexec="/tmp/limelight toolexec" .
```

Then, from another shell:

```
curl -sS -XPOST localhost:8080/debug/limelight/enable \
  -d '{"version":0,"ttl":"10s","match":{"projectID":"abc-123"}}'
```

```
created widget
created gizmo
{"time":"...","level":"INFO","msg":"limelight","version":0,
 "method":"main.Service.CreateThing","fields":{"project.id":"abc-123","user.id":"u-42"}}
created widget
created gizmo
```

Only `abc-123` emits — `zzz-999` runs through the same tagged method and stays silent.
Ten seconds later the switch turns itself off and the events stop with it.

`GET /debug/limelight/status` reports whether it is on and until when;
`POST /debug/limelight/disable` stops it early.

## Run it without the shim

```
go run .
```

The service behaves identically and emits nothing: `//limelight:method` is an ordinary
comment, and the compiler ignores it. That silence is the failure mode the vet analyzer
exists to catch — and the analyzer is not built yet, so for now the only signal is
`LIMELIGHT_VERBOSE=1`, which makes the shim report each method it instruments.
