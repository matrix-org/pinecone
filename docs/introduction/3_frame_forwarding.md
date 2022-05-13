---
title: Frame Forwarding
parent: Introduction
nav_order: 3
permalink: /introduction/frame_forwarding
---

// TODO : Watermark!

# Frame Forwarding

When receiving an incoming data frame on a port, the switch then uses the correct lookup method for the frame type to determine which port to forward the frame to.

Pinecone separates frames into two types: **protocol frames** and **traffic frames**.

Protocol frames often have specific rules governing their behaviour, including inspecting and processing the protocol control messages at intermediate or destination hops, and which routing scheme should be used to forward them onwards.

Traffic frames, on the other hand, are always forwarded using SNEK routing or tree routing (typically the former) and are not required to be otherwise inspected by an intermediate node.

If a suitable next-hop is identified, the frame will be forwarded to the chosen next-hop peer.
