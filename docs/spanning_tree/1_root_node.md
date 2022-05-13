---
title: Root Node
parent: Spanning Tree
nav_order: 1
permalink: tree/root_node
---

# Root Node

As with all spanning trees, there is a single root node at any given time for a given network. It is the responsibility of the root node to send out root announcements into the network at a specific interval.

Since Pinecone networks are typically dynamic in nature, root selection must take place automatically and without user intervention, so as to avoid deliberate points of centralisation. In order to do this, a form of network-wide election takes place, where the node with the numerically highest ed25519 public key will win.

If the current root node disappears from the network or otherwise stops sending root announcements (due to dropped peerings), another election will take place, eventually settling on the next highest key.

A node considers itself to be a root node if it does not have a chosen parent.
