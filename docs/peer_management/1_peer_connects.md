---
title: Peer Connects
parent: Peer Management
nav_order: 1
permalink: /peer/connect
---

# Peer Connects

When a new peer connects to a node, the node should immediately send the latest root update from the chosen parent. If no chosen parent is available or the node is acting as a root, the node should send its own root update immediately.
