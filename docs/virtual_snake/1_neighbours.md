---
title: Snake Neighbours
parent: Virtual Snake
nav_order: 1
permalink: /virtual_snake/neighbours
---

# Snake Neighbours

Each node in the topology has a reference to a descending path. The  descending path is the path on the network that leads to the next closest public key to our own in the descending direction (the next lowest key).

There is only one exception to this rule: the node with the lowest key on the network will have only an ascending path (as there is no lower key to build a descending path).

For the descending path, a node should store the following information:

1. The **Origin public key**, noting which node initiated the path creation;
2. The **Source port**, where the Bootstrap message arrived from;
3. The **Destination port**, where the Bootstrap message was forwarded to next (if applicable);
4. The **Last seen** time, noting when the entry was populated;
5. The **Root public key** and **Root sequence** that the path was set up with.
6. The **Watermark public key** and **Watermark sequence** that the path was set up with.
