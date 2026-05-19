package testutil

import (
	"os"
	"path/filepath"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// WriteDemoEchoDescriptorSet writes a minimal proto descriptor set used by JSON<->gRPC transcoding tests.
func WriteDemoEchoDescriptorSet(dir string) (string, error) {
	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("demo/echo.proto"),
		Package: proto.String("demo.v1"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("EchoRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("name"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
					{
						Name:     proto.String("user_id"),
						JsonName: proto.String("userId"),
						Number:   proto.Int32(2),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
					{
						Name:   proto.String("age"),
						Number: proto.Int32(3),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
					},
				},
			},
			{
				Name: proto.String("EchoResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("message"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
					{
						Name:   proto.String("ok"),
						Number: proto.Int32(2),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum(),
					},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("EchoService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("Echo"),
						InputType:  proto.String(".demo.v1.EchoRequest"),
						OutputType: proto.String(".demo.v1.EchoResponse"),
					},
				},
			},
		},
	}

	set := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{file},
	}

	data, err := proto.Marshal(set)
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, "demo_echo_descriptor.pb")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
