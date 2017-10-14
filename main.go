package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	pb "github.com/tcolgate/test/grpcbuffer/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	client bool
	server bool
)

func runClient() {
	creds, err := credentials.NewClientTLSFromFile("cert.pem", "")
	if err != nil {
		log.Fatalf("fail to get creds: %v", err)
	}
	conn, err := grpc.Dial("localhost:8888", grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()
	client := pb.NewTestClient(conn)

	t, err := client.Tail(context.Background(), &pb.TailRequest{})
	if err != nil {
		log.Fatalf("err in Tail call, %v", err)
	}

	for {
		tr, err := t.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			log.Printf("err: %v", err)
			return
		}

		fmt.Printf("Got tr = %v , err = %v\n", tr.Count, err)
	}
}

type testImp struct{}

func (t *testImp) Tail(tr *pb.TailRequest, s pb.Test_TailServer) error {
	ctx := s.Context()

	i := 0
	data := make([]byte, 4096)
Loop:
	for {
		select {
		case <-ctx.Done():
			break Loop
		default:
			log.Println("Sending")
			rand.Read(data)
			err := s.Send(&pb.Message{Count: uint64(i), Data: data})
			if err != nil {
				log.Printf("Error: %v", err)
				return err
			}
			time.Sleep(1 * time.Second)
		}
		i++
	}

	return nil
}

func runServer() {
	lis, err := net.Listen("tcp", "localhost:443")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	creds, err := credentials.NewServerTLSFromFile("cert.pem", "key.pem")
	if err != nil {
		log.Fatalf("Failed to generate credentials %v", err)
	}
	grpcServer := grpc.NewServer(grpc.Creds(creds))

	ts := &testImp{}
	pb.RegisterTestServer(grpcServer, ts)
	grpcServer.Serve(lis)

}

func main() {
	flag.BoolVar(&client, "c", false, "run client")
	flag.BoolVar(&server, "s", false, "run server")
	flag.Parse()

	switch {
	case client:
		runClient()
	case server:
		runServer()
	}
}
