package main

import (
	"context"
	dictionary "github.com/Hexfall/DISYSExam/Dictionary"
	"google.golang.org/grpc"
	"log"
	"sync"
	"time"
)

type Server struct {
	dictionary.UnimplementedIncrementServiceServer

	isLeader       bool
	leaderAddr     string
	leaderConn     *grpc.ClientConn
	leaderClient   dictionary.IncrementServiceClient
	selfAddr       string
	replicas       []string
	replicaMutex   sync.Mutex
	replicaConns   map[string]*grpc.ClientConn
	replicaClients map[string]dictionary.IncrementServiceClient

	value int64
	mutex sync.Mutex
}

func CreateClient(ip string) (*grpc.ClientConn, dictionary.IncrementServiceClient) {
	var options []grpc.DialOption
	options = append(options, grpc.WithBlock(), grpc.WithInsecure(), grpc.WithTimeout(3*time.Second))
	log.Printf("Attempting to establish connection with ip %s...\n", ip)
	conn, err := grpc.Dial(ip, options...)
	if err != nil {
		log.Fatalf("Failed to dial gRPC server on ip %s. Error: %v", ip, err)
	} else {
		log.Printf("Successfully connected to %s\n", ip)
	}

	client := dictionary.NewIncrementServiceClient(conn)
	return conn, client
}

// Leader Methods.

func (s *Server) BroadcastReplicas() {
	for _, cli := range s.replicaClients {
		go s.SendReplicasToReplica(cli)
	}
}

func (s *Server) BroadcastValue() {
	for _, cli := range s.replicaClients {
		go s.SendValueToReplica(cli)
	}
}

func (s *Server) SendReplicasToReplica(client dictionary.IncrementServiceClient) {
	_, err := client.SendReplicas(context.Background(), &dictionary.ReplicaListMessage{Ips: s.replicas})
	if err == context.DeadlineExceeded {
		// Timed out, attempting to send replicas to replica.
		// TODO: Do something.
	} else if err != nil {
		log.Fatalf("Failed to send replicas to client. Error: %v", err)
	}
}

func (s *Server) SendValueToReplica(client dictionary.IncrementServiceClient) {
	_, err := client.SendValue(context.Background(), &dictionary.IncrementMessage{Number: s.value})
	if err == context.DeadlineExceeded {
		// Timed out, attempting to send replicas to replica.
		// TODO: Do something. Or leave alone and let HeartBeat monitor take care of it, eventually.
	} else if err != nil {
		log.Fatalf("Failed to send replicas to client. Error: %v", err)
	}
}

func removeElement(arr []string, elem string) []string {
	for i, e := range arr {
		if e == elem {
			return append(arr[:i], arr[i+1:]...)
		}
	}

	// Return unchanged, if element not found.
	return arr
}

func (s *Server) HeartbeatMonitor(ip string, client dictionary.IncrementServiceClient) {
	var err error = nil
	for err == nil {
		// Runs on a loop, checking for replica heartbeat. Exits loop upon receiving an error.
		time.Sleep(1 * time.Second)
		_, err = client.HeartBeat(context.Background(), &dictionary.VoidMessage{})
	}
	if err != nil {
		if err != context.DeadlineExceeded {
			log.Printf("Encountered unexepected error while listening to heartbeat. Error: %v", err)
		}
		// replica is unresponsive/has exceeded their deadline.
		if s.isLeader {
			// TODO: Kill connection.
			s.replicaMutex.Lock()

			s.replicaConns[ip].Close()
			delete(s.replicaConns, ip)
			delete(s.replicaClients, ip)
			s.replicas = removeElement(s.replicas, ip)

			s.replicaMutex.Unlock()
			log.Printf("Disconnected sub-replica at ip %s\n", ip)
			go s.BroadcastReplicas()
		} else {
			// TODO: Check whether replica is new leader.
			// TODO: Otherwise, connect to new leader.
			log.Println("Cluster has been decapitated. Reconfiguring...")
			if s.replicas[0] == s.selfAddr {
				log.Println("Self is first in succession.")
				// This replica is first in succession order, and assumes leadership.
				s.leaderAddr = ""
				s.isLeader = true
				// Clear replica list. Other replicas will reconnect.
				s.replicas = []string{}
				log.Println("Reconfigured as leader.")
			} else {
				s.leaderConn.Close()
				log.Println("Giving new leader time to reconfigure...")
				time.Sleep(5 * time.Second)
				s.leaderAddr = s.replicas[0]
				log.Println("Connecting to new leader...")
				s.JoinCluster()
			}
		}
	}
}

// Sub-replica methods.

func (s *Server) JoinCluster() {
	if s.isLeader {
		log.Fatalln("Leader node attempted to join cluster. Investigate.")
	}

	conn, cli := CreateClient(s.leaderAddr)
	mes, err := cli.GetLeader(context.Background(), &dictionary.VoidMessage{})
	if err != nil {
		log.Fatalf("Failed to retrieve leader information from %s. Error: %v", s.leaderAddr, err)
	}
	if mes.IsLeader {
		s.leaderConn = conn
		s.leaderClient = cli

		_, err := cli.Join(context.Background(), &dictionary.IpMessage{Ip: s.selfAddr})
		if err != nil {
			log.Fatalf("Failed to join cluster. Error: %v", err)
		}

		go s.HeartbeatMonitor("", cli)
	} else {
		s.leaderAddr = mes.Ip
		s.JoinCluster()
	}
}

// gRPC functions.

func (s *Server) Increment(ctx context.Context, void *dictionary.VoidMessage) (*dictionary.IncrementMessage, error) {
	if !s.isLeader {
		// This replica doesn't have the authority to Increment.
		return nil, &dictionary.ImpermissibleError{}
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.value++
	log.Printf("Value has been incremented to %d.\n", s.value)
	go s.BroadcastValue()
	return &dictionary.IncrementMessage{Number: s.value}, nil
}

func (s *Server) GetLeader(ctx context.Context, void *dictionary.VoidMessage) (*dictionary.LeaderMessage, error) {
	return &dictionary.LeaderMessage{
		Ip:       s.leaderAddr,
		IsLeader: s.isLeader,
	}, nil
}

func (s *Server) GetReplicas(ctx context.Context, void *dictionary.VoidMessage) (*dictionary.ReplicaListMessage, error) {
	return &dictionary.ReplicaListMessage{Ips: s.replicas}, nil
}

func (s *Server) Join(ctx context.Context, ipMessage *dictionary.IpMessage) (*dictionary.VoidMessage, error) {
	if !s.isLeader {
		return nil, &dictionary.ImpermissibleError{}
	}

	s.replicaMutex.Lock()

	s.replicas = append(s.replicas, ipMessage.Ip)
	conn, cli := CreateClient(ipMessage.Ip)
	s.replicaConns[ipMessage.Ip] = conn
	s.replicaClients[ipMessage.Ip] = cli

	s.replicaMutex.Unlock()

	// Send replicas and value.
	go s.BroadcastReplicas()
	go s.SendValueToReplica(cli)
	// Monitor heartbeat.
	go s.HeartbeatMonitor(ipMessage.Ip, cli)

	return &dictionary.VoidMessage{}, nil
}

func (s *Server) HeartBeat(ctx context.Context, void *dictionary.VoidMessage) (*dictionary.VoidMessage, error) {
	return &dictionary.VoidMessage{}, nil
}

func (s *Server) SendReplicas(ctx context.Context, replicasMessage *dictionary.ReplicaListMessage) (*dictionary.VoidMessage, error) {
	if s.isLeader {
		// Leader cannot be ordered around.
		return nil, &dictionary.ImpermissibleError{}
	}
	// Should really check whether sender is leader, somehow.
	s.replicas = replicasMessage.Ips

	return &dictionary.VoidMessage{}, nil
}

func (s *Server) SendValue(ctx context.Context, incrementMessage *dictionary.IncrementMessage) (*dictionary.VoidMessage, error) {
	if s.isLeader {
		// Leader cannot be ordered around.
		return nil, &dictionary.ImpermissibleError{}
	}
	// Should really check whether sender is leader, somehow.
	s.value = incrementMessage.Number
	log.Printf("Value has been set to %d by leader.\n", incrementMessage.Number)

	return &dictionary.VoidMessage{}, nil
}
