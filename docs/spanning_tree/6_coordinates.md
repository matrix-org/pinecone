---
title: Coordinates
parent: Spanning Tree
nav_order: 6
permalink: /tree/coordinates
---

# Coordinates

A node’s coordinates describe the location of the node on the spanning tree. In effect, the coordinates are the path from the root node down to a given node, with each number representing the downward port number at each hop.

For example, coordinates `[1 4 2 4]` describe the following path: starting via the root node’s port 1, followed by the next node’s port 4, followed by the next node’s port 2 and then, finally, the destination node’s parent’s port 4.

The coordinates of the node are calculated by taking the chosen parent’s last root announcement and concatenating all of the **Destination port number** values in order.

With that in mind, a root node, which has no chosen parent, will have a zero-length set of coordinates `[]` and a node’s coordinates will change each time it either selects a new parent, or receives a root announcement update from the parent with a different path described in the signatures.
