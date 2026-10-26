package malformed

import (
	"context"

	_ "github.com/stuparm/limelight/limelight-go"
)

// NoFields has prose above its tag, and the diagnostic still lands on the
// directive's own line.
// want +2 `missing fields`
//
//limelight:method
func NoFields(ctx context.Context) {}

// want +2 `unknown argument`
//
//limelight:method scope:"all"
func UnknownArgument(ctx context.Context) {}

// want +2 `no space allowed after fields:`
//
//limelight:method fields: "projectID"
func SpaceAfterColon(ctx context.Context) {}

// want +2 `no space allowed before the colon`
//
//limelight:method fields :"projectID"
func SpaceBeforeColon(ctx context.Context) {}

// want +2 `not a double-quoted string`
//
//limelight:method fields:projectID
func Unquoted(ctx context.Context) {}

// want +2 `trailing text "emits the project"`
//
//limelight:method fields:"projectID" emits the project
func Trailing(ctx context.Context) {}

// want +2 `field "projectID" listed twice`
//
//limelight:method fields:"projectID,projectID"
func Twice(ctx context.Context) {}

// want +3 `a second //limelight:method in this doc comment is ignored`
//
//limelight:method fields:"projectID"
//limelight:method fields:userID
func Second(ctx context.Context) {}
