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

Traffic frames, on the other hand, are always forwarded using SNEK routing or tree routing and are not required to be otherwise inspected by an intermediate node.

If a suitable next-hop is identified, the frame will be forwarded to the chosen next-hop peer.

## Hybrid Traffic Routing

**Traffic frames** are forwarded using a combination of both SNEK routing and tree routing. This is done to reduce the overall network stretch while still providing a reliable transport mechanism during times of high network churn.

Traffic is initially routed using SNEK routing to the destination and the frame header is populated with the sending node's tree coordinates. Upon reaching the destination, the receiving node caches the sending node's tree coordinates.

When sending traffic, a node will check for matching cached tree coordinates and populate that information in the frame header if possible. The presence of destination coordinates in a traffic frame lets other nodes know to attempt forwarding using tree routing.

When forwarding traffic frames using tree routing, if any node along the path fails to find a suitable next-hop then the destination coordinates should be removed from the header and SNEK forwarding should be attempted to reach the destination.

To assist with adjusting to changing network conditions, individual cached coordinates should be removed if they ever become too old. Also, all cached coordinates should be removed if the tree root changes. 

## Watermarks

In the case of **traffic frames** there is also a watermark used to help detect routing loops. Watermarks only factor into the equation when next-hops are selected from the SNEK routing table as these routes can become stale much more quickly than route information obtained through the global spanning tree. The watermark ensures that forward progress towards a given key is being made and that a packet will never be forwarded onto a path that is worse than the path selected at the last hop. If any node along the path does not know of either the same best destination node or a closer destination node, then the packet has reached a location where further forwarding could result in routing loops.

Frames should be dropped if the path watermark (derived from the path key and bootstrap sequence) of the chosen next-hop is worse than the watermark on the received frame. A watermark is defined as being worse if either of the following conditions is met:
- The new watermark has a higher public key than the existing watermark;
- The new watermark has the same public key but a lower sequence number than the existing watermark;

Before forwarding the frame to the next-hop, if the chosen next-hop was selected using the snake routing table then the watermark on the frame should be updated with the path watermark of the next-hop.
