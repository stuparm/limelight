package limelight

import "testing"

// The two shapes the rewriter could generate, measured against each other. The
// difference is what the guard is for.
func BenchmarkUnguardedEmitWhileOff(b *testing.B) {
	ctx := benchSetup()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Emit(ctx, "p.S.M", "projectID", "userID")
	}
}

func BenchmarkGuardedEmitWhileOff(b *testing.B) {
	ctx := benchSetup()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if On() {
			Emit(ctx, "p.S.M", "projectID", "userID")
		}
	}
}
