---
title: Root Election
parent: Spanning Tree
nav_order: 7
permalink: /tree/root_election
---

# Root Election

When a node first comes online, it has no prior knowledge about the state of the network and therefore does not have a chosen parent. Therefore each node that comes online and joins the network will start off considering itself to be a root node.

When the first peer connection is made, the node should send its root announcement to the newly connected peer and wait for the remote peer to send their announcement back. One of two things will happen:

1. Our key will be stronger than the remote side’s root key (in which case we will remain as a root node, they will have received good news about a new stronger root and will re-run the parent selection algorithm accordingly), or:
2. The root announcement that they send us will contain a stronger root key than our own public key (which we will consider to be good news) which will cause us to run parent selection.

In a network which is entirely starting up from cold, or in the case of the previous root node disappearing/going offline, the process of agreeing on a new root is iterative. Nodes will run the parent selection algorithm, finding the strongest root key, notifying their peers, which may cause them to run the parent selection algorithm, announcing their strongest chosen root to their peers, and so on, until the network eventually settles on the strongest key.
