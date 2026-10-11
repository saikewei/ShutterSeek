// Package creationrpc defines the small protobuf Struct RPC transport.
package creationrpc

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

const MaxMessage = 24 << 20

type Workspace interface {
	Call(context.Context, *structpb.Struct) (*structpb.Struct, error)
}

func RegisterWorkspace(s *grpc.Server, w Workspace) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "shutterseek.creation.v1.Workspace", HandlerType: (*Workspace)(nil),
		Methods: []grpc.MethodDesc{{MethodName: "Call", Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
			in := new(structpb.Struct)
			if err := dec(in); err != nil {
				return nil, err
			}
			invoke := func(ctx context.Context, req interface{}) (interface{}, error) {
				return srv.(Workspace).Call(ctx, req.(*structpb.Struct))
			}
			if interceptor == nil {
				return invoke(ctx, in)
			}
			return interceptor(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/shutterseek.creation.v1.Workspace/Call"}, invoke)
		}}},
	}, w)
}
func Execute(ctx context.Context, conn *grpc.ClientConn, input map[string]interface{}, receive func(map[string]interface{}) error) error {
	request, err := structpb.NewStruct(input)
	if err != nil {
		return err
	}
	stream, err := conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/shutterseek.creation.v1.Agent/Execute", grpc.MaxCallRecvMsgSize(MaxMessage))
	if err != nil {
		return err
	}
	if err = stream.SendMsg(request); err != nil {
		return err
	}
	if err = stream.CloseSend(); err != nil {
		return err
	}
	for {
		out := new(structpb.Struct)
		if err = stream.RecvMsg(out); err != nil {
			return err
		}
		if err = receive(out.AsMap()); err != nil {
			return err
		}
	}
}
