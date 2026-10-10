package creationrpc

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/structpb"
	"io"
	"net"
	"testing"
)

type echoWorkspace struct{}

func (echoWorkspace) Call(ctx context.Context, in *structpb.Struct) (*structpb.Struct, error) {
	return in, nil
}
func TestWorkspaceStructWireContract(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	RegisterWorkspace(server, echoWorkspace{})
	go server.Serve(listener)
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	request, _ := structpb.NewStruct(map[string]interface{}{"op": "checkpoint_put", "payload": map[string]interface{}{"photo_ids": []interface{}{float64(1)}}})
	out := new(structpb.Struct)
	if err := conn.Invoke(context.Background(), "/shutterseek.creation.v1.Workspace/Call", request, out); err != nil {
		t.Fatal(err)
	}
	if out.AsMap()["op"] != "checkpoint_put" {
		t.Fatal("wire mismatch")
	}
}

type testAgent interface{}

func TestExecuteServerStream(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	server.RegisterService(&grpc.ServiceDesc{ServiceName: "shutterseek.creation.v1.Agent", HandlerType: (*testAgent)(nil), Streams: []grpc.StreamDesc{{StreamName: "Execute", ServerStreams: true, Handler: func(_ interface{}, stream grpc.ServerStream) error {
		in := new(structpb.Struct)
		if err := stream.RecvMsg(in); err != nil {
			return err
		}
		for _, stage := range []string{"rendering", "reviewed"} {
			out, _ := structpb.NewStruct(map[string]interface{}{"stage": stage})
			if err := stream.SendMsg(out); err != nil {
				return err
			}
		}
		return nil
	}}}}, struct{}{})
	go server.Serve(listener)
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	count := 0
	err = Execute(context.Background(), conn, map[string]interface{}{"run_id": "test"}, func(event map[string]interface{}) error { count++; return nil })
	if err != io.EOF || count != 2 {
		t.Fatalf("stream: count=%d err=%v", count, err)
	}
}
