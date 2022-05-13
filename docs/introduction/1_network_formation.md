---
title: Network Formation
parent: Introduction
nav_order: 1
permalink: /introduction/network_formation
---

# Network Formation

Strictly speaking, a Pinecone network is formed every time two or more nodes connect to each other. Any two networks that are joined together by a common node will merge into a single larger network.

Even though the Pinecone address space is considered to be global in scope, it is important to note that there is no true “single” Pinecone network. It is possible for multiple disjoint and disconnected Pinecone network segments to exist.

This does mean that if every node globally is connected somehow to the same mesh, Pinecone will converge on a single global network and all nodes will be routable to each other as a result.

It is possible to create private/closed networks within trusted environments by enforcing mutual authentication of peering connections, although this is an implementation-specific detail and not specified here. This may be desirable in some particularly sensitive settings to ensure that trusted nodes do not participate in untrusted networks and that untrusted nodes do not participate in trusted networks.
