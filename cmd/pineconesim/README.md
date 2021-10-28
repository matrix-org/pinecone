# Pinecone Simulator

The Pinecone Simulator is a tool used to test Pinecone nodes in various scenarios. These scenarios include both fixed topologies and node mobility. 

## Running

From the top-level Pinecone directory, run the following:
```go run cmd/pineconesim/main.go```

## Simulator UI

To access the simulator's interface, visit `localhost:65432` in your web browser.

## Development

### Design Goals

The overall design goal of the simulator is to be able to handle running a large number of Pinecone nodes while still being responsive. 

It is preferable that the simulator can be run offline and without any external build or runtime dependencies. Having a simulator that is extremely simple to run and modify makes it easier for more developers to test Pinecone, thus making Pinecone better in return.

The simulator UI runs in the web browser. This is done for a few reasons:
- web browsers are ubiquitous
- cross platform by default
- can easily connect to a remotely running simulator
- multiple developers can have a view into the same simulator instance

### Detailed Design

In order to decouple the effects of running the simulator from the performance of the Pinecone node/s, the simulator works by sending/receiving state change information over channels to/from each Pinecone node. When receiving information, the relevant state is cached locally in the simulator.

The simulator UI works using a websocket to the running simulator. Upon accessing the simulator instance in the browser, a websocket is established that the simulator can use to forward state change events up to the UI. The first message received from the simulator will contain the current state of the entire simulation. After that, the simulator will only send state update messages in the order that they occurred. The websocket can also be used for the UI to request changes to the running simulation by sending messages down to the running simulator.

```
        +--------------+
        | UI / Browser |
        +--------------+
               ⇅ (websocket)
+-------------------------------+
|           Simulator           |
|          ⤢        ⤡ (chan)   |
| +----------+     +----------+ |
| | Pinecone | ... | Pinecone | |
| |   Node   |     |   Node   | |
| +----------+     +----------+ |
+-------------------------------+
```
