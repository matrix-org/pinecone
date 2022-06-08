---
title: Bootstrapping
parent: Virtual Snake
nav_order: 2
permalink: /virtual_snake/bootstrapping
---

# Bootstrapping

Bootstrapping is the process of joining the snake topology. Bootstrapping takes place in three steps:

1. The bootstrapping node sends a bootstrap message into the network, with their own public key as the “target” key, which will be routed to the nearest keyspace neighbour;
2. The nearest keyspace neighbour will respond to the bootstrap message by sending back a Bootstrap ACK;
3. The bootstrapping node will respond to the Bootstrap ACK by sending a Path Setup message to the nearest keyspace neighbour.

Nodes bootstrap when they do not have an ascending path — that is, they do not know who the next highest public key belongs to. The descending path is populated passively by another node bootstrapping and building a path to that node.

The bootstrap message contains the following fields:

<table>
  <tr>
   <td colspan="2" >Source coordinates
   </td>
  </tr>
  <tr>
   <td>Path public key
   </td>
   <td>Path ID
   </td>
  </tr>
  <tr>
   <td>Root public key
   </td>
   <td>Root sequence
   </td>
  </tr>
  <tr>
   <td colspan="2" >Source signature
   </td>
  </tr>
</table>

The combination of the **Path public key** and the **Path ID** uniquely identify a path. While the **Path public key** is predetermined by the public key of the bootstrapping node, the **Path ID** must be generated randomly by the bootstrapping node and should not be reused.

The **Source signature** is an ed25519 signature covering both the **Path public key** and **Path ID** by concatenating them together and signing the result. This enables the remote side to verify that the bootstrap was genuinely initiated by the sending node and has not been forged.

Bootstraps will travel through the network, forwarded **using SNEK routing with bootstrap rules**, towards the target key, until they arrive at the node that is closest to the target key. The forwarding logic specifically will not deliver bootstrap messages to the actual target key, so the bootstrap message will eventually arrive at a “dead end” at the next closest key.
