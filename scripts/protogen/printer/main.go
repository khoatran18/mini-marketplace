// Command printer writes buf/validate/validate.proto to stdout, reconstructed from the descriptor that is
// compiled into the protovalidate Go module. It lets scripts/protogen/gen.sh run `buf generate` when
// buf.build (where the proto dependency normally lives) is not reachable.
package main

import (
	"os"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoprint"
)

func main() {
	fd, err := desc.WrapFile(validate.File_buf_validate_validate_proto)
	if err != nil {
		panic(err)
	}
	if err := (&protoprint.Printer{}).PrintProtoFile(fd, os.Stdout); err != nil {
		panic(err)
	}
}
