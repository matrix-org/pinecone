---
title: Path Teardown
parent: Virtual Snake
nav_order: 6
permalink: /virtual_snake/teardown
---

# Path Teardown

A teardown is a special type of protocol message that signals to a node that a path is no longer valid and should be removed. A teardown message contains the following fields:

<table>
  <tr>
   <td>Path public key
   </td>
   <td>Path ID
   </td>
  </tr>
</table>

If the teardown message arrived from any port that was not the **Source port** or the **Destination port**, the teardown must be dropped and ignored. A valid teardown will only ever arrive from the same ports as the original path was built on.

Upon receipt of a path teardown message that matches a routing table entry and arrives from either the **Source port** or the **Destination port**, the node should clear any routing table, ascending or descending path entries that match the **Path public key** and **Path ID**.

Once the teardown has been actioned, it must be forwarded based on the following rules:

1. If the teardown arrived via the **Source port**, and the **Destination port** is not empty, forward it to the **Destination port**;
2. If the teardown arrived via the **Destination port**, and the **Source port** is not empty, forward it to the **Source port**.

A teardown that originates locally must be forwarded to all related ports — that is, if both the **Destination port** and **Source port** are known, a teardown must be sent to each port.

If a received teardown message does not match any routing table entries and/or the ascending or descending entry, the teardown should be ignored and dropped, and must not be forwarded.

In the case that the teardown message results in the ascending path being torn down, the node should then re-bootstrap by sending a new bootstrap message into the network.
