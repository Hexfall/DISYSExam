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
	dictionary.UnimplementedDictionaryServiceServer

	isLeader       bool
	leaderAddr     string
	leaderConn     *grpc.ClientConn
	leaderClient   dictionary.DictionaryServiceClient
	selfAddr       string
	replicas       []string
	replicaMutex   sync.Mutex
	replicaConns   map[string]*grpc.ClientConn
	replicaClients map[string]dictionary.DictionaryServiceClient

	values map[int32]int32
	mutex  sync.Mutex
}

func (s *Server) ContainsKey(key int32) bool {
	for k, _ := range s.values {
		if k == key {
			return true
		}
	}
	return false
}

func (s *Server) GetValue(key int32) int32 {
	if s.ContainsKey(key) {
		return s.values[key]
	} else {
		return 0
	}
}

func CreateClient(ip string) (*grpc.ClientConn, dictionary.DictionaryServiceClient) {
	var options []grpc.DialOption
	options = append(options, grpc.WithBlock(), grpc.WithInsecure(), grpc.WithTimeout(3*time.Second))
	log.Printf("Attempting to establish connection with ip %s...\n", ip)
	conn, err := grpc.Dial(ip, options...)
	if err != nil {
		log.Fatalf("Failed to dial gRPC server on ip %s. Error: %v", ip, err)
	} else {
		log.Printf("Successfully connected to %s\n", ip)
	}

	client := dictionary.NewDictionaryServiceClient(conn)
	return conn, client
}

// Leader Methods.

func (s *Server) BroadcastReplicas() {
	for _, cli := range s.replicaClients {
		go s.SendReplicasToReplica(cli)
	}
}

func (s *Server) BroadcastValue(key int32, value int32) {
	for _, cli := range s.replicaClients {
		go s.SendValueToReplica(cli, key, value)
	}
}

func (s *Server) BroadcastValues() {
	for _, cli := range s.replicaClients {
		go s.SendValuesToReplica(cli)
	}
}

func (s *Server) SendReplicasToReplica(client dictionary.DictionaryServiceClient) {
	_, err := client.SendReplicas(context.Background(), &dictionary.ReplicaListMessage{Ips: s.replicas})
	if err == context.DeadlineExceeded {
		// Timed out, attempting to send replicas to replica.
		// TODO: Do something.
	} else if err != nil {
		log.Fatalf("Failed to send replicas to client. Error: %v", err)
	}
}

func (s *Server) SendValueToReplica(client dictionary.DictionaryServiceClient, key int32, value int32) {
	_, err := client.SendValue(context.Background(), &dictionary.PutMessage{
		Key:   key,
		Value: value,
	})
	if err == context.DeadlineExceeded {
		// Timed out, attempting to send replicas to replica.
		// TODO: Do something. Or leave alone and let HeartBeat monitor take care of it, eventually.
	} else if err != nil {
		log.Fatalf("Failed to send value to client. Error: %v", err)
	}
}

func (s *Server) SendValuesToReplica(client dictionary.DictionaryServiceClient) {
	var values []*dictionary.PutMessage
	for k, v := range s.values {
		values = append(values, &dictionary.PutMessage{
			Key:   k,
			Value: v,
		})
	}
	_, err := client.SendValues(context.Background(), &dictionary.PutAllMessage{Values: values})
	if err == context.DeadlineExceeded {
		// Timed out, attempting to send replicas to replica.
		// TODO: Do something. Or leave alone and let HeartBeat monitor take care of it, eventually.
	} else if err != nil {
		log.Fatalf("Failed to send values to client. Error: %v", err)
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

func (s *Server) HeartbeatMonitor(ip string, client dictionary.DictionaryServiceClient) {
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

func (s *Server) Get(ctx context.Context, getMessage *dictionary.GetMessage) (*dictionary.ValueMessage, error) {
	log.Printf("Received request for value of %d. Returning %d.\n", getMessage.Key, s.GetValue(getMessage.Key))
	return &dictionary.ValueMessage{Value: s.GetValue(getMessage.Key)}, nil
}

func (s *Server) Put(ctx context.Context, putMessage *dictionary.PutMessage) (*dictionary.SuccessMessage, error) {
	if !s.isLeader {
		return &dictionary.SuccessMessage{Success: false}, &dictionary.ImpermissibleError{}
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	log.Printf("Received request to set value of %d to %d.\n", putMessage.Key, putMessage.Value)
	s.values[putMessage.Key] = putMessage.Value
	go s.BroadcastValue(putMessage.Key, putMessage.Value)

	return &dictionary.SuccessMessage{Success: true}, nil
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

	log.Printf("Request to join cluster on ip %s received...\n", ipMessage.Ip)
	s.replicas = append(s.replicas, ipMessage.Ip)
	conn, cli := CreateClient(ipMessage.Ip)
	s.replicaConns[ipMessage.Ip] = conn
	s.replicaClients[ipMessage.Ip] = cli
	log.Printf("%s has been added to the cluster.\n", ipMessage.Ip)

	s.replicaMutex.Unlock()

	// Send replicas and value.
	log.Printf("Forwarding cluster information to new replica on ip %s...\n", ipMessage.Ip)
	go s.BroadcastReplicas()
	go s.SendValuesToReplica(cli)
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
	s.replicaMutex.Lock()
	defer s.replicaMutex.Unlock()

	s.replicas = replicasMessage.Ips

	return &dictionary.VoidMessage{}, nil
}

func (s *Server) SendValue(ctx context.Context, putMessage *dictionary.PutMessage) (*dictionary.VoidMessage, error) {
	if s.isLeader {
		// Leader cannot be ordered around.
		return nil, &dictionary.ImpermissibleError{}
	}
	// Should really check whether sender is leader, somehow.
	s.mutex.Lock()
	defer s.mutex.Unlock()
	log.Printf("Value of %d set to %d by leader.\n", putMessage.Key, putMessage.Value)
	s.values[putMessage.Key] = putMessage.Value

	return &dictionary.VoidMessage{}, nil
}

func (s *Server) SendValues(ctx context.Context, putAllMessage *dictionary.PutAllMessage) (*dictionary.VoidMessage, error) {
	if s.isLeader {
		// Leader cannot be ordered around.
		return nil, &dictionary.ImpermissibleError{}
	}
	// Should really check whether sender is leader, somehow.
	log.Printf("Receiving values from leader.\n")
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.values = make(map[int32]int32)
	for _, valuePair := range putAllMessage.Values {
		s.values[valuePair.Key] = valuePair.Value
	}

	return &dictionary.VoidMessage{}, nil
}
