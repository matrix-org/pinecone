---
title: Handling Bootstraps
parent: Virtual Snake
nav_order: 3
permalink: /virtual_snake/bootstraps
---

# Handling Bootstrap Messages

Once the bootstrap message arrives at a dead end, the node will update it's descending node entry if it makes sense to do so (ie. same **Root public key** and **Root sequence** and a closer key than the previous descending entry or an update from our existing descending node).

Before doing anything, the node must ensure that the signature in the **Source signature** field is valid by checking against the **Destination public key**. If the signature is invalid, the bootstrap message should be silently dropped and not processed any further.

The node should ensure that the **Root public key** and **Root sequence** of the bootstrap message match those of the most recent root announcement from our chosen parent, if any, or the node’s own public key and sequence number if the node is currently acting as a root node. If this is not true, the bootstrap message should be silently dropped and not processed any further.

Each node along the bootstrapping path should install the bootstrapping node into their routing table.

## Install route into routing table

Regardless of whether the bootstrap message is considered to have arrived at its intended destination (there is no closer node to route to) or not, a bootstrap message should result in the route being installed into the routing table of each node handling the message.

Before installing the bootstrapping node into the routing table, each node should compare the bootstrap sequence against any existing entry for the bootstrapping node. If an entry exists and the new bootstrap sequence number isn't higher than the current entry, then the bootstrap should be dropped and not processed any further. The only reason to see a bootstrap with a lower or equal sequence number to a bootstrap the node has seen before is if there is a routing loop present.

To install the route into the routing table, the node should either create a new entry or overwrite the existing entry and:

1. Copy the **Origin public key** into the virtual snake index;
2. Copy the bootstrap **Root public key** and **Root sequence** into the appropriate fields;
3. Populate the **Last seen time** with the current time;
4. Populate the **Source port** with a reference to the port that the setup message was received from;
5. Populate the **Destination port** with a reference to the chosen next-hop port, unless the bootstrap message has reached its intended destination, in which case this field should remain empty;
6. Copy the setup **Watermark public key** and **Watermark sequence** into the appropriate fields.
