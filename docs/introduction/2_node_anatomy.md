---
title: Node Anatomy
parent: Introduction
nav_order: 2
permalink: /introduction/node_anatomy
---

# Node Anatomy

A Pinecone node is effectively a form of user-space hybrid router/switch. The switch has a number of “ports” which can connect to other Pinecone nodes using stream-oriented and reliable transports (such as TCP connections).

Each port is numbered. There is no requirement for a Pinecone node to have a specific number of ports. However, switch port 0 is always reserved for the local Pinecone router. Traffic going to and from the local node will always use this port. Port number assignment is an implementation-specific detail, although it typically makes sense to always assign the lowest free port number.

The router maintains state that allows it to participate in the network, including:

* Information about all directly connected peers, including their public key, last tree announcement received, how they are connected to us etc;
* Which of the node’s directly connected peers is our chosen parent in the tree, if any;
* A routing table, containing information about SNEK paths that have been set up through this node by other nodes;
* Information about our ascending and descending keyspace neighbours — that is, which node has the next highest (ascending) and next lowest (descending) key to the node’s own;
* A sequence number, used when operating as a root node and sending the nodes own root announcements into the network;
* Maintenance timers for tree and SNEK maintenance.
