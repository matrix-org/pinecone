---
title: Handling Bootstraps
parent: Virtual Snake
nav_order: 3
permalink: /virtual_snake/bootstraps
---

# Handling Bootstrap Messages

Once the bootstrap message arrives at a dead end, the node will respond using **tree routing** back to the node with a Bootstrap ACK message.

Before doing anything, the node must ensure that the signature in the **Source signature** field is valid by checking against the **Destination public key**. If the signature is invalid, the bootstrap message should be silently dropped and not processed any further.

Additionally, the node should ensure that the **Root public key** and **Root sequence** of the bootstrap message match those of the most recent root announcement from our chosen parent, if any, or the node’s own public key and sequence number if the node is currently acting as a root node. If this is not true, the bootstrap message should be silently dropped and not processed any further.

A Bootstrap ACK message contains the following fields:

<table>
  <tr>
   <td>Destination coordinates
   </td>
   <td>Source coordinates
   </td>
  </tr>
  <tr>
   <td>Destination public key
   </td>
   <td>Source public key
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

1. Copy the bootstrap **Source coordinates** into the **Destination coordinates** field;
2. Copy the bootstrap **Path public key** into the **Destination public key** field;
3. Copy the bootstrap **Path ID** into the **Path ID** field;
4. Copy the bootstrap **Source signature** into the **Source signature** field;
5. Populate the node’s own current coordinates into the **Source coordinates** field;
6. Populate the node’s own public key into the **Source public key** field;
7. Copy their own parent’s last root announcement **Root public key** and **Root sequence** fields into the corresponding fields;
8. Add the **Destination signature**, as below.

The **Destination signature** is a signature that covers the **Source signature**, **Path public key** and **Path ID** fields by concatenating all three values together and then signing the result. It enables any node on the network to verify that the acknowledging node accepted a specific bootstrap for a specific path.

Note that the **Source signature** is copied and preserved from the original packet without modification. This is so that the bootstrap ACK contains signatures from both ends of the bootstrap.
