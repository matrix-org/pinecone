---
title: Root Announcements
parent: Spanning Tree
nav_order: 2
permalink: /tree/root_announcements
---

# Root Announcements

It is necessary for the functioning of a Pinecone network for all nodes to hear an announcement from the same root node so that all nodes can agree on the same tree-space coordinate system for routing bootstrap and setup messages.

A root announcement contains the following fields:

<table>
  <tr>
   <td colspan="2" >Root public key
   </td>
  </tr>
  <tr>
   <td colspan="2" >Root sequence
   </td>
  </tr>
  <tr>
   <td rowspan="3" >Signature 1
   </td>
   <td>Signing public key
   </td>
  </tr>
  <tr>
   <td>Destination port number
   </td>
  </tr>
  <tr>
   <td>Signature covering all fields upto this signature
   </td>
  </tr>
  <tr>
   <td rowspan="3" >Signature 2
   </td>
   <td>Signing public key
   </td>
  </tr>
  <tr>
   <td>Destination port number
   </td>
  </tr>
  <tr>
   <td>Signature covering all fields upto this signature
   </td>
  </tr>
  <tr>
   <td rowspan="3" >Signature …n
   </td>
   <td>Signing public key
   </td>
  </tr>
  <tr>
   <td>Destination port number
   </td>
  </tr>
  <tr>
   <td>Signature covering all fields upto this signature
   </td>
  </tr>
</table>

#### Sequence number

The sequence number is a monotonically increasing unsigned integer, starting at 0. Since root announcements are flooded through the network, it is the goal for all nodes to eventually agree on the same **Root public key** and **Sequence number** following a root update.

The sequence number **must** be increased when the root node wishes to send out a new root announcement by reaching the announcement time interval.

The sequence number **must not** be increased when repeating the previous update to peers for any other reason, and must not be increased by any node that is not the root node before repeating the updates to their peers.

#### Signatures

Before sending the root announcement to a peer, it should sign the update, appending the signature to the end of the announcement. Doing so allows any node receiving an root announcement to ensure that none of the ancestors have forged earlier signatures within the update, and to more reliably detect loops within the tree (if a signature incorrectly appears within a given update more than once).

Note that the **Destination port number** field must include the port number of the peer that the update is being sent to. Therefore if you are repeating the update to 6 peers, you will need to create 6 separate signed updates.
