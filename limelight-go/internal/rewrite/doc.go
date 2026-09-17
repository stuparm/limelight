// Package rewrite is the AST codemod: it finds tagged methods and injects the
// generated gate + emit call. Source rewrite, not -toolexec — golang/go#69887
// may move the ground under toolexec-based tools.
package rewrite
