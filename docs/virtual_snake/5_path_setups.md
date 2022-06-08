---
title: Handling Path Setups
parent: Virtual Snake
nav_order: 5
permalink: /virtual_snake/setups
---

# Handling Path Setup Messages

Path Setup messages are responsible for populating the routing table, in addition to updating the reference to the descending node when the message reaches its final destination. They are different to Bootstrap and Bootstrap ACK messages in that they must be processed by all intermediate nodes on the path before being forwarded.

Importantly, if a node decides that it wants to reject a setup message for any reason, it **must** send a teardown for the new path to ensure that it is cleaned up, as it will have possibly been installed into the routing table of other nodes on the path. This includes if the setup message cannot be forwarded to its destination due to reaching a dead end.

If the setup message has reached its intended destination, that is that the **Destination public key** matches the node’s public key, then the responding node should decide whether or not to update their descending reference to the new path.

If an entry in the routing table already exists with the **Destination public key** and the **Path ID**, the routing entry is a duplicate:

1. The new path must be torn down;
2. The old path must be torn down;
3. The update must be rejected and not processed any further.

## Arrived at intended destination

If the **Destination public key** is equal to the node’s public key, the update is considered to have arrived at its intended destination, therefore the following checks should be performed as to whether or not to update the descending node reference:

1. Drop the update and do not process further if any of the following are true:
    1. If the **Root public key** does not match that of our chosen parent’s last announcement;
    2. If the **Root sequence** does not match that of our chosen parent’s last announcement;
    3. The **Source public key** is not less than the node’s own public key;
2. If the node already has a descending entry, and it has not expired:
    1. If the **Source public key** is the same as the existing descending entry’s public key, and the **Path ID** is different to the existing descending entry’s **Path ID**, accept the update;
    2. If the ordering **Descending public key ＜ Source public key ＜ Node public key** is true, that is that the public key that the setup came from is closer to us in keyspace than our previous descending node, accept the update;
3. If the node does not have a descending entry, or the node has an expired descending entry:
    1. If the **Source public key** is less than the node’s own public key, accept the update.

If the update has not been accepted, a teardown of the new path must be sent back via the receiving port and the update should be dropped.

If the update has been accepted, the node’s descending reference should be populated to point to the node from which the Setup message arrived from:

1. Copy the setup **Source public key** into both the **Path public key** and **Origin public key** fields;
2. Copy the setup **Path ID** into the **Path ID** field;
3. Copy the setup **Root public key** and **Root sequence** into the appropriate fields;
4. Populate the **Last seen time** with the current time;
5. Populate the **Source port** with a reference to the port that the setup message was received from;
6. Leave the **Destination port** empty, as there should be no next-hop once the setup message has been processed at its intended destination.

Then proceed into the next section to install the route into the routing table.

## Install route into routing table

Regardless of whether the setup message is considered to have arrived at its intended destination (the node’s public key matches the **Destination public key**) or not, a setup message should result in the route being installed into the routing table of each node handling the message.

In the event that the setup message is due to be forwarded (i.e. the setup message has not yet reached its intended destination), installing the routing table entry should be done **after** the message has been forwarded.

By doing this, all transitive nodes on a given setup path will contain routing information for the newly built path. There is one of three possible outcomes:

1. The intended destination for the setup message will accept the route, therefore the route will remain up;
2. The intended destination will reject the route, sending back a teardown along the path, causing the routing table entry to be deleted;
3. The setup message will never arrive at the intended destination, instead hitting a dead end, with the node at the dead end sending back a teardown along the path, causing the routing table entry to be deleted.

To install the route into the routing table, the node should create a new entry and:

1. Copy the setup **Source public key** into both the **Path public key** and **Origin public key** fields;
2. Copy the setup **Path ID** into the **Path ID** field;
3. Copy the setup **Root public key** and **Root sequence** into the appropriate fields;
4. Populate the **Last seen time** with the current time;
5. Populate the **Source port** with a reference to the port that the setup message was received from;
6. Populate the **Destination port** with a reference to the chosen next-hop port, unless the setup message has reached its intended destination, in which case this field should remain empty.
