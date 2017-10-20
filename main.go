package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/codahale/hdrhistogram"
	pb "github.com/tcolgate/grpcbuffer/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	client     bool
	pingClient bool
	server     bool
	size       int
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

func runPingClient() {
	wg := &sync.WaitGroup{}
	vals := make(chan time.Duration)
	hg := hdrhistogram.New(0, 1000000000, 5)

	go func() {
		tick := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-tick.C:
				fmt.Printf("count: %d mean: %s 4*9s: %s stddev: %s\n",
					hg.TotalCount(),
					time.Nanosecond*time.Duration(hg.Mean()),
					time.Nanosecond*time.Duration(hg.ValueAtQuantile(99.99)),
					time.Nanosecond*time.Duration(hg.StdDev()),
				)
			case v := <-vals:
				hg.RecordValue(int64(v / time.Nanosecond))
			}
		}
	}()

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
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
			for {
				n := time.Now()
				_, err := client.Ping(context.Background(), &pb.PingRequest{})
				if err != nil {
					log.Fatalf("err in Tail call, %v", err)
				}
				d := time.Since(n)
				vals <- d
			}
		}()
	}
	wg.Wait()
}

type testImp struct{}

var pingCount uint64

func (t *testImp) Tail(tr *pb.TailRequest, s pb.Test_TailServer) error {
	ctx := s.Context()

	i := 0
	data := make([]byte, size)
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

func (t *testImp) Ping(ctx context.Context, tr *pb.PingRequest) (*pb.Message, error) {
	return &pb.Message{
		Count: atomic.AddUint64(&pingCount, 1),
	}, nil
}

func main() {
	flag.BoolVar(&client, "c", false, "run client")
	flag.BoolVar(&pingClient, "C", false, "run ping client")
	flag.BoolVar(&server, "s", false, "run server")
	flag.IntVar(&size, "z", 512, "padding size")
	flag.Parse()

	switch {
	case client:
		runClient()
	case pingClient:
		runPingClient()
	case server:
		runServer()
	}
}
