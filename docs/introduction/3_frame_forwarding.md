---
title: Frame Forwarding
parent: Introduction
nav_order: 3
permalink: /introduction/frame_forwarding
---

# Frame Forwarding

When receiving an incoming data frame on a port, the switch then uses the correct lookup method for the frame type to determine which port to forward the frame to.

Pinecone separates frames into two types: **protocol frames** and **traffic frames**.

Protocol frames often have specific rules governing their behaviour, including inspecting and processing the protocol control messages at intermediate or destination hops, and which routing scheme should be used to forward them onwards.

Traffic frames, on the other hand, are always forwarded using SNEK routing or tree routing (typically the former) and are not required to be otherwise inspected by an intermediate node.

If a suitable next-hop is identified, the frame will be forwarded to the chosen next-hop peer.

## Watermarks

In the case of **traffic frames** there is also a watermark used to help detect routing loops. Watermarks only factor into the equation when next-hops are selected from the SNEK routing table as these routes can become stale much more quickly than route information obtained through the global spanning tree. The watermarks are designed to track the closest destination node information that any given node along the route that a packet traverses knows about. If any node along the route does not know of either the same best destination node or a closer destination node, then the packet has reached a location where further forwarding could result in routing loops.

Frames should be dropped if the watermark returned from the next-hop algorithm is worse than the watermark on the received frame. A watermark is defined as being worse if either of the following conditions is met:
- The new watermark has a higher public key than the existing watermark;
- The new watermark has the same public key but a lower sequence number than the existing watermark;

Before forwarding the frame to the next-hop, if the watermark returned from the next-hop algorithm has a sequence number higher than 0 then the watermark on the frame should be updated with the newly calculated watermark. If the newly calculated watermark has a sequence number of 0 then the next-hop candidate wasn't selected from the SNEK routing table so it shouldn't be used to update the frame.
