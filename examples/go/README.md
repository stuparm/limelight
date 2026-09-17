# Go example

```
go run .
```

Panics until `Register` is implemented — it is the development target, not a demo.

It is also the fixture: `CreateThing` carries a real `//limelight:method` directive, so the
codemod (`limelight -w ./...`) and the vet analyzer have something to chew on from the
first commit.
