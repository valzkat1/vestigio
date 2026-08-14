package mcp

// Version is reported in the MCP initialize handshake and by `vestigio version`.
//
// A var, not a const, and that is the whole reason this file has a comment.
// `-ldflags -X` can only write to a string VARIABLE — a const is folded into the
// binary at compile time and is unreachable from the linker. As a const, every
// release would have carried whatever number was last edited by hand, and the
// day someone forgot, a published binary would have reported a version it was
// not. That is the quiet wrong answer this project is built against, shipped to
// other people's machines.
//
// The release workflow overwrites it from the git tag:
//
//	-X github.com/valzkat1/vestigio/internal/mcp.Version=1.2.3
//
// The value below is what a `go build` or `go install` from source reports, and
// says so on purpose: a binary built from a working tree is not a release, and
// a bug report should not be able to claim it was.
var Version = "dev"
