module github.com/stuparm/limelight/limelight-go/limelightzap

go 1.23

replace github.com/stuparm/limelight/limelight-go => ..

require (
	github.com/stuparm/limelight/limelight-go v0.0.0-00010101000000-000000000000
	go.uber.org/zap v1.28.0
)

require go.uber.org/multierr v1.10.0 // indirect
