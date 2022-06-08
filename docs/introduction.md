---
title: Introduction
has_children: true
nav_order: 1
permalink: /introduction
---

# Introduction

Pinecone is an implemenatation of the Sequentially Networked Edwards Key (SNEK) name-independent routing scheme that combines two network topologies in order to build an efficient and scalable overlay network.

Pinecone is:

* **Name-independent**:
    * Node identifiers are not related to the physical network topology;
    * They do not change as the node moves around the network;
    * The address space is considered to be global — there is no subnetting;

* **Built up of equal players**:
    * Nodes will forward traffic on behalf of other nodes;
    * There are no specific points of centralisation;
    * A node can join a larger network by connecting to any other node;

* **Self-healing**:
    * Tolerates node mobility far better than Batman-adv, Babel and many other routing protocols;
    * Alternative paths will be discovered automatically if possible when nodes go offline or connections are lost.

* **Transport-agnostic**:
    * The only requirements for a peering today are that it is stream-oriented and reliable;
    * Any connection medium that can offer these semantics is appropriate for a peering, including TCP over regular IP networks, WebSockets, Bluetooth L2CAP (with infinite retransmit) or RFCOMM etc.

Note, however, that Pinecone is:

* **Not anonymous**:
    * Anonymity networks make significant trade-offs in order to achieve anonymity, both in overall complexity and in path/routing cost, which are unacceptable in resource-constrained environments;
    * It is not a goal of the Matrix project to guarantee anonymity — at best Matrix can provide pseudonymity and the same is true with Pinecone public keys;

* **Not strongly resistant to traffic analysis**:
    * Although traffic packet contents will be encrypted end-to-end by default, the source and destination fields in the packet headers are visible to intermediate nodes;
    * In some cases, source addresses could be sealed/encrypted, although this is not implemented today;

* **Not fully resilient against malicious nodes**:
    * If a node peers with malicious nodes which (for instance) drop traffic, then connectivity will inevitably be disrupted. This can never be fully mitigated (for instance, if you are in a 2 node network and the other node is malicious, there is nothing you can do) — but work is ongoing to detect and route around malicious behaviour where possible.

The Pinecone routing scheme effectively is built up of two major components:

* A **global spanning tree**:
    * Provides efficient, low-stretch and strictly loop-free routing;
    * Effective means of exchanging path setup messages, even before SNEK bootstrap has taken place;

* A **virtual snake** (or **SNEK**) — a double-linked linear routing topology:
    * Provides resilient public key-based routing across the overlay network.
