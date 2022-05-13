---
title: Parent Selection
parent: Spanning Tree
nav_order: 5
permalink: /tree/parent_selection
---

# Parent Selection

With the exception of the root node, which cannot by definition have a parent, each node on the network must choose one of its peered nodes to be its “parent”. The chosen parent node will influence the node’s coordinates.

In the case that a non-root node has only a single peering, that peered node will always be the chosen parent. In the case there are multiple peerings, the parent selection algorithm will determine which the most suitable node is.

The parent selection algorithm runs as follows:

1. Start with the current **Root public key** and **Root sequence** as the <span style="text-decoration:underline;">best key</span> and <span style="text-decoration:underline;">best sequence</span>, and the <span style="text-decoration:underline;">best candidate</span> as an empty field;
2. Iterate through all directly connected peers, checking all of the following conditions in order:
    1. Skip the peer and move onto the next if the last root announcement received from this peer exceeds the announcement timeout interval, which is typically **45 minutes**;
    2. Skip the peer and move onto the next if the last root announcement received from this peer includes our own public key in the **Signing public key** field of any of the signatures, as this implies that we are receiving an update again that we have already seen and that a routing loop has taken place;
    3. If the **Root public key** of the last peer update is higher than the best key, update the best key and mark this peer as the best candidate;
    4. If the **Root public key** of the last peer update is lower than the best key, skip the peer and move onto the next;
    5. If the **Root sequence** of the last peer update is higher than the best sequence, update the best sequence and mark this peer as the best candidate;
    6. If the **Root sequence** of the last peer update is lower than the best sequence, skip the peer and move onto the next;
    7. As a last resort tie-breaker, mark this peer as the best candidate and move onto the next if the peer's **Root public key** and **Root sequence** match the best key and best sequence, and either:
        * This peer's update arrived before the update from the best candidate did, or;
        * No other peer has been selected as the best candidate so far.

If the best candidate is not empty at the end of the loop and the chosen candidate is different to the current chosen parent, mark the best candidate as our chosen parent and then send root announcement updates to all directly connected peers.

If the best candidate is still empty at the end of the algorithm, none of the connected peers were suitable candidates and therefore the node should become a root node instead, emptying the chosen parent field and then sending a root announcement to all directly connected peers with its own public key as the **Root public key**.

Directly connected peers are likely to handle the sudden update to a weaker root key as bad news, therefore any nodes that were previously children of this node will respond accordingly.
