package nearmiss

import (
	"context"

	_ "github.com/stuparm/limelight/limelight-go"
)

// Each mistake below is written the way gofmt saves it: a doc-comment line
// that is not directive-shaped gets a space after its //, and a
// directive-shaped one moves to the end of the comment, behind a blank //.

// want +1 `no space allowed between // and limelight, so this is an ordinary comment`
// limelight:method fields:"projectID"
func SpaceAfterSlashes(ctx context.Context) {}

// want +1 `no space allowed between // and limelight; no space allowed around the colon`
// limelight: method fields:"projectID"
func SpaceAfterColon(ctx context.Context) {}

// want +1 `directive names are lower-case`
// Limelight:method fields:"projectID"
func UpperCase(ctx context.Context) {}

// want +1 `lime-light:method is misspelled`
// lime-light:method fields:"projectID"
func HyphenatedNamespace(ctx context.Context) {}

// want +2 `unknown directive //limelight:metod — did you mean //limelight:method`
//
//limelight:metod fields:"projectID"
func VerbTypo(ctx context.Context) {}

// want +2 `//limelght:method is not a limelight directive`
//
//limelght:method fields:"projectID"
func NamespaceTypo(ctx context.Context) {}

// want +2 `missing space after //limelight:method`
//
//limelight:methodfields:"projectID"
func Glued(ctx context.Context) {}

// want +2 `//limelight:type is not implemented`
//
//limelight:type
type T struct{}

// want +1 `must be a // line comment`
/* limelight:method fields:"projectID" */
func Block(ctx context.Context) {}

// Beside is tagged, and a near-miss next to its tag is still reported.
//
// want +3 `unknown directive //limelight:metod`
//
//limelight:method fields:"projectID"
//limelight:metod fields:"userID"
func Beside(ctx context.Context) {}

// A near-miss in no doc comment at all is reported too.

// want +1 `no space allowed around the colon`
//limelight: method fields:"projectID"

func Floating(ctx context.Context) {}
