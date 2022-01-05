package main

import (
	"context"
	dictionary "github.com/Hexfall/DISYSExam/Dictionary"
	"google.golang.org/grpc"
	"log"
	"time"
)

type FrontEnd struct {
	client    dictionary.DictionaryServiceClient
	conn      *grpc.ClientConn
	ctx       context.Context
	backups   []string
	connected bool
}

func (fe *FrontEnd) Get(key int) int {
	mes, err := fe.client.Get(fe.ctx, &dictionary.GetMessage{Key: int32(key)})
	if err != nil {
		log.Printf("Encountered error whilst requesting value for %d. Error: %v", key, err)
		fe.LostConnection()
		return fe.Get(key)
	}

	go fe.GetReplicas()
	return int(mes.Value)
}

func (fe *FrontEnd) Set(key int, value int) bool {
	mes, err := fe.client.Put(fe.ctx, &dictionary.PutMessage{
		Key:   int32(key),
		Value: int32(value),
	})
	if err != nil {
		log.Printf("Encountered error whilst assigning value %d for %d. Error: %v", value, key, err)
		fe.LostConnection()
		return fe.Set(key, value)
	}

	go fe.GetReplicas()
	return mes.Success
}

func (fe *FrontEnd) LostConnection() {
	log.Println("Assuming crash on leader node. Attempting to reconnect to cluster at other entry-point.")
	fe.Connect(fe.backups[0])
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
	fe.client = dictionary.NewDictionaryServiceClient(fe.conn)

	// Find the leader of the cluster.
	log.Println("Querying connected replica for cluster leader.")
	mes, err := fe.client.GetLeader(fe.ctx, &dictionary.VoidMessage{})
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
	mes, err := fe.client.GetReplicas(fe.ctx, &dictionary.VoidMessage{})
	if err != nil {
		log.Fatalf("Failed to retrieve replicas from leader. Error: %v", err)
	}
	fe.backups = mes.Ips
}
