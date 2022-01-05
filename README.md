# DISYS Final Exam (Distributed Hash Table)
## How to run
There are two components of the system, which need to be run in separate ways. As this is a distributed system, you (the user) are expected to run multiple nodes of each of these two components.
### Replica
```
go run ./Replica/ -port {int} - target {string}
```
The replica *is* the system. It is through them that access to the system is gained, and they are where data is stored and retrieved from.

There are two types of replica: Leader and Follower.

The Replica node has two flags:
 - `port | int | 5080 by default` Is the gRPC port which the node listens through.
 - `target | string | "" by default` Designates an address where the node expects to find a cluster to join.
If this flag is an empty string (as it is by default), the node instead designates itself leader of a new cluster.  
The system, in its current implementation, is reliant on connections being made locally (same machine)
and, since the program is written in go, addresses should be written in the format ":{port}" (example ":5080").

### Client
```
go run ./Client/ -target {string}
```
The client is the node through which, a user can interact with the system. It takes input during runtime, and acts in accordance.
Many clients can (and should, for the purpose of testing) run simultaneously.

It has one flag:
 - `target | string | ":5080" by default` This is an address, where the front-end expects to find a replica which belongs to a cluster (either leader or follower). It follows the same conventions as the `target` flag of the replica.