package main

import (
	"context"
	increment "github.com/Hexfall/DISYSMockExam/Increment"
	"google.golang.org/grpc"
	"log"
	"time"
)

type FrontEnd struct {
	client    increment.IncrementServiceClient
	conn      *grpc.ClientConn
	ctx       context.Context
	backups   []string
	connected bool
}

func (fe *FrontEnd) Increment() int64 {
	mes, err := fe.client.Increment(fe.ctx, &increment.VoidMessage{})
	if err != nil {
		log.Printf("Failed to increment value. Error: %v", err)
		log.Println("Assuming connection lost to cluster leader.")
		if len(fe.backups) > 0 {
			log.Println("Attempting to reconnect to cluster through other entry-point.")
			fe.Connect(fe.backups[0])
			return fe.Increment()
		} else {
			log.Fatalln("No other replicas known. Terminating program.")
		}
	}
	go fe.GetReplicas()
	return mes.Number
}

func (fe *FrontEnd) Connect(ip string) {
	log.Println("Connecting to new Replica")
	if fe.connected {
		log.Println("Closing previous connection.")
		fe.conn.Close()
	}

	var options []grpc.DialOption
	options = append(options, grpc.WithBlock(), grpc.WithInsecure(), grpc.WithTimeout(3*time.Second))
	log.Printf("Attempting to establish connection with ip %s...\n", ip)
	conn, err := grpc.Dial(ip, options...)
	if err == context.DeadlineExceeded {
		// TODO idk
		log.Fatalf("Connection timed out.\n")
	} else if err != nil {
		log.Fatalf("Failed to dial gRPC server on ip %s. Error: %v", ip, err)
	} else {
		log.Printf("Successfully connected to %s\n", ip)
	}
	fe.conn = conn
	fe.connected = true

	fe.ctx = context.Background()
	fe.client = increment.NewIncrementServiceClient(fe.conn)

	// Find the leader of the cluster.
	log.Println("Querying connected replica for cluster leader.")
	mes, err := fe.client.GetLeader(fe.ctx, &increment.VoidMessage{})
	if err != nil {
		log.Fatalf("Failed to get leader. Error %v", err)
	}
	if !mes.IsLeader {
		// If not currently connected to the Leader of the cluster, change replica which is connected.
		log.Println("Retrieved new leader from connected replica. Changing connection.")
		fe.Connect(mes.Ip)
	} else {
		log.Println("Connected replica is cluster leader.")
		fe.GetReplicas()
	}
}

func (fe *FrontEnd) GetReplicas() {
	mes, err := fe.client.GetReplicas(fe.ctx, &increment.VoidMessage{})
	if err != nil {
		log.Fatalf("Failed to retrieve replicas from leader. Error: %v", err)
	}
	fe.backups = mes.Ips
}
