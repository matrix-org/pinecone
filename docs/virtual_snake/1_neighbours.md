---
title: Snake Neighbours
parent: Virtual Snake
nav_order: 1
permalink: /virtual_snake/neighbours
---

# Snake Neighbours

Each node in the topology has a reference to an ascending and a descending path. The ascending path is the path on the network that leads to the closest public key to our own in the ascending direction (the next highest key), and the descending path is the path on the network that leads to the next closest public key to our own in the descending direction (the next lowest key).

There are only two exceptions to this rule: the node with the highest key on the network will have only a descending path (as there is no higher key to build an ascending path to) and the node with the lowest key on the network will have only an ascending path (as there is no lower key to build a descending path).

For the ascending and descending paths, a node should store the following information:

1. The **Path public key** and **Path ID**, as exchanged during bootstrap/path setup;
2. The **Origin public key**, noting which node initiated the path creation;
3. The **Source port**, where the Bootstrap ACK message arrived from (for descending path entries);
4. The **Destination port**, where the Path Setup message was forwarded to next (for ascending path entries);
5. The **Last seen** time, noting when the entry was populated;
6. The **Root public key** and **Root sequence** that the path was set up with.
