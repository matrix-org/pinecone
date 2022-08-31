---
title: Bootstrapping
parent: Virtual Snake
nav_order: 2
permalink: /virtual_snake/bootstrapping
---

# Bootstrapping

Bootstrapping is the process of joining the snake topology. Bootstrapping takes place every 5 seconds and takes place in two steps:

1. The bootstrapping node sends a bootstrap message into the network, with their own public key as the “destination” key, which will be routed to the nearest keyspace neighbour;
2. The nearest keyspace neighbour will add the bootstrapping node as their descending neighbour.

The bootstrap message contains the following fields:

<table>
  <tr>
   <td colspan="2" >Bootstrap Sequence
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

The **Source signature** is an ed25519 signature covering the **Bootstrap Sequence**, **Root public key** and **Root Sequence** by concatenating them together and signing the result. This enables the remote side to verify that the bootstrap was genuinely initiated by the sending node and has not been forged.

Bootstraps will travel through the network, forwarded **using SNEK routing with bootstrap rules**, towards the destination key, until they arrive at the node that is closest to the destination key. The forwarding logic specifically will not deliver bootstrap messages to the actual destination key, so the bootstrap message will eventually arrive at a “dead end” at the next closest key.
