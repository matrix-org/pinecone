---
title: Peer Disconnects
parent: Peer Management
nav_order: 2
permalink: /peer/disconnect
---

# Peer Disconnects

A Pinecone node must immediately remove any state related to a peer that disconnects, including the last received root announcement from that peer.

If the chosen parent is the disconnected peer, the node must re-run the parent selection algorithm immediately, either selecting a new parent (with the equivalent **Root public key** and **Root sequence**) or by becoming a root node and waiting for a stronger update from a peer.

If the disconnected peer appears in any entry in the routing table, as either the **Source port** or **Destination port**, a teardown must be sent to the remaining port. For example, if the **Source port** is now disconnected, a teardown for the path must be sent to the **Destination port**. The entry should then be removed from the routing table.

If the disconnected port is the **Destination port** of the ascending node entry, the ascending entry should be cleared. There is no **Source port** for an ascending node entry, therefore it is not possible to send a teardown for this path. The node on the other side of the failed connection is responsible for sending a teardown in this case.

If the disconnected port is the **Source port** of the descending node entry, the descending entry should be cleared. There is no **Destination port** for a descending node entry, therefore it is not possible to send a teardown for this path. The node on the other side of the failed connection is responsible for sending a teardown in this case.

The next iteration of the routine maintenance should send a bootstrap message into the network if the ascending node entry was cleared.
