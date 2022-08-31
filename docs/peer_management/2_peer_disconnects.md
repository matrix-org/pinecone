---
title: Peer Disconnects
parent: Peer Management
nav_order: 2
permalink: /peer/disconnect
---

# Peer Disconnects

A Pinecone node must immediately remove any state related to a peer that disconnects, including the last received root announcement from that peer.

If the chosen parent is the disconnected peer, the node must re-run the parent selection algorithm immediately, either selecting a new parent (with the equivalent **Root public key** and **Root sequence**) or by becoming a root node and waiting for a stronger update from a peer.

If the disconnected peer appears in any entry in the routing table, as either the **Source port** or **Destination port**, the entry should be removed from the routing table.

If the disconnected port is the **Source port** of the descending node entry, the descending entry should be cleared.
