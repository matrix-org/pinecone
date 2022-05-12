---
title: Handling Bootstrap ACKs
parent: Virtual Snake
nav_order: 4
permalink: /virtual_snake/bootstrap_acks
---

# Handling Bootstrap ACK Messages

Upon receipt of a Bootstrap ACK packet, the bootstrapping node now is considered to have “found” a potential ascending node candidate.

The Bootstrap ACK packet will contain two signatures: the **Source signature** and the **Destination signature**. Both of these signatures should be verified to ensure authenticity. The **Source signature** must be verified using the node’s own public key and the **Destination signature** must be verified using the **Source public key**. If either signature is invalid, the packet should be dropped and not processed any further.

If the signature is valid, the following checks should be made to see if it is suitable:

1. Drop the update and do not process further if any of the following are true:
    1. If the **Source public key** is the same as the node’s own public key, implying that the node somehow received a Bootstrap ACK from itself;
    2. If the **Root public key** does not match that of our chosen parent’s last announcement;
    3. If the **Root sequence** does not match that of our chosen parent’s last announcement;
2. If the node already has an ascending entry, and it has not expired:
    1. If the **Source public key** is the same as the existing ascending entry’s public key, and the **Path ID** is different to the existing ascending entry’s path ID, accept the update;
    2. If the ordering **Node public key ＜ Source public key ＜ Ascending origin public key** is true, that is that the public key that the Bootstrap ACK came from is closer to us in keyspace than our previous ascending node, accept the update;
3. If the node does not have an ascending entry, or the node has an expired ascending entry:
    1. If the **Source public key** is greater than the node’s own public key, accept the update.

If the update has not been accepted, it should be dropped. There is no new path to tear down and it is not necessary to respond to the bootstrapping node.

If the update has been accepted, a Path Setup message is constructed. A Path Setup message contains the following fields:

<table>
  <tr>
   <td>Destination public key
   </td>
   <td>Destination coordinates
   </td>
  </tr>
  <tr>
   <td colspan="2" >Source public key
   </td>
  </tr>
  <tr>
   <td colspan="2" >Path ID
   </td>
  </tr>
  <tr>
   <td>Root public key
   </td>
   <td>Root sequence
   </td>
  </tr>
  <tr>
   <td>Source signature
   </td>
   <td>Destination signature
   </td>
  </tr>
</table>

The responding node should:

1. Copy the bootstrap ACK **Source public key** into the **Destination public key** field;
2. Copy the bootstrap ACK **Source coordinates** into the **Destination coordinates** field;
3. Populate the node’s own public key into the **Source public key** field;
4. Copy the bootstrap ACK’s **Path ID** into the **Path ID** field;
5. Copy their own parent’s last root announcement **Root public key** and **Root sequence** fields into the corresponding fields;
6. Copy the bootstrap ACK’s **Source signature** into the **Source signature** field;
7. Copy the bootstrap ACK’s **Destination signature** into the **Destination signature** field.

Path setup messages are routed using **tree routing** — that is, the **Destination coordinates** are the prominent field when making routing decisions. The **Destination public key** field is used to confirm that the setup has reached its intended destination.

Each path setup message will contain both a **Source signature** and a **Destination signature**. The **Source signature** must be verified using the **Source public key** and the **Destination signature** must be verified using the **Destination public key**. If either of the signatures is invalid, the setup message should be dropped and a teardown must be sent back for the new path to the port that the setup message was received on.

The node should attempt to look up the next hop for the message and then attempt to forward the Path Setup onto the first hop. If this fails, either because there is no suitable next-hop identified or because the packet was not successfully deliverable to the first hop, the setup message should be dropped and a teardown must be sent back for the new path to the port that the setup message was received on.

The path is only useful if we can assert that it arrived at the intended destination and was set up correctly at all intermediate nodes without being torn down at any point. Since no routing information has been installed yet, there is nothing to tear down, so dropping is sufficient to abort the path setup altogether.

If the Path Setup was successfully forwarded, the node’s ascending reference should be populated to point to the node from which the Bootstrap ACK arrived from:

1. Copy the bootstrap ACK **Destination public key** into the **Path public key** field;
2. Copy the bootstrap ACK **Path ID** into the **Path ID** field;
3. Copy the bootstrap ACK **Source public key** into the **Origin public key** field;
4. Copy the bootstrap ACK **Root public key** and **Root sequence** into the appropriate fields;
5. Populate the **Last seen** time with the current time;
6. Populate the **Source** port with a reference to the port that the bootstrap ACK was received from;
7. Populate the **Destination** port with a reference to the chosen next-hop port, from above.

Finally, the node must then iterate through the routing table and search for all entries where the **Source** port refers to nowhere/the local node, and the **Path public key** does not match the bootstrap ACK **Source public key**, sending teardowns for each of these paths and removing them from the routing table. This ensures that any stale paths from other nodes are torn down, but it will not remove paths from the newly bootstrapping node until it has received the Path Setup and torn down the path itself, avoiding a race condition. Teardowns are described in a later section.
