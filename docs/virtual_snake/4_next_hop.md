---
title: Next Hop Calculation
parent: Virtual Snake
nav_order: 4
permalink: /virtual_snake/nexthop
---

**TODO: Watermark Info**

# Next Hop Calculation

When using SNEK routing to route towards a certain public key, a number of rules apply in order to calculate the next-hop.

These rules slightly differ based on whether the frame is considered to be a “bootstrap” message. Only “Bootstrap” frames follow the bootstrap rules.

1. Start with a <span style="text-decoration:underline;">best key</span> set to the node’s public key, and a <span style="text-decoration:underline;">best candidate</span> set to the node’s own router port;
2. If the **Destination public key** is equal to the node’s own public key and the frame is not a bootstrap message, handle the packet locally without forwarding;
3. If the node has a chosen parent (i.e. is not a root node) and an announcement has been received from that parent:
    1. If the frame is a bootstrap message and the best key still equals the node’s public key, which should always be the case to begin with, ensure that a worst-case route up to the root is chosen:
        - Set the best key to the chosen parent’s root public key;
        - Set the best candidate to the port through which the parent is reachable;
    2. If the ordering **Best key ＜ Destination public key ＜ Root public key** is true, implying that the target is higher in keyspace than our own key, ensure that a worst-case route up to the root is chosen:
        - Set the best key to the chosen parent’s root public key;
        - Set the best candidate to the port through which the parent is reachable;
    3. For each of the node’s ancestors — that is, public keys that appear in the **Signatures** section of the last received root update from the chosen parent:
        1. If the frame is not a bootstrap message, the candidate ancestor key equals the **Destination public key** and the best key does not equal the **Destination public key**, meaning that we know that the target is one of our ancestors:
            - Set the best key to the ancestor key;
            - Set the best candidate to the port through which the parent is reachable;
        2. If the ordering **Destination public key ＜ Ancestor key ＜ Best key** is true, meaning that we believe one of our ancestors takes us closer to the target:
            - Set the best key to the ancestor key;
            - Set the best candidate to the port through which the parent is reachable;
4. For each of the node’s directly connected peers (first loop):
    1. For each of the connected peer’s ancestors — that is, public keys that appear in the **Signatures** section of the last received root update from this peer:
        1. If the frame is not a bootstrap message, the candidate peer ancestor key equals the **Destination public key** and the best key does not equal the **Destination public key**, meaning that we believe that the target is one of our direct peer’s ancestors:
            - Set the best key to the ancestor key;
            - Set the best candidate to the port through which the peer is reachable;
5. For each of the node’s directly connected peers (second loop):
    1. If the best key equals the connected peer’s public key, i.e. we have previously found the peer’s key as an ancestor of another node but not using the most direct port, we can now refine the path to use the direct connection to that peer instead:
        - Set the best key to the peer’s public key;
        - Set the best candidate to the port through which the peer is reachable;
6. For each of our routing table entries, to look for any transitive paths that may take the packet closer to the target than any of our direct peering knowledge has provided us:
    1. Skip the routing table entry if either of the following conditions are true:
        - The routing table entry has expired;
        - The **Source port** of the routing table entry refers to the local router;
    2. If the frame is not a bootstrap message, the **Path public key** is equal to the **Destination public key** and the best key is not equal to the **Destination public key**:
        - Set the best key to the **Path public key**;
        - Set the best candidate to the **Source port** from the entry;
    3. If the ordering **Destination public key ＜ Path public key ＜ Best key** is true:
        - Set the best key to the **Path public key**;
        - Set the best candidate to the **Source port** from the entry.

If none of the above conditions have matched for the given **Destination public key**, then it is expected that the best candidate will still refer to the local router port, in which case the node is expected to handle the traffic as if it was destined for the local node.
