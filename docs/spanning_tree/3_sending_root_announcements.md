---
title: Sending Root Announcements
parent: Spanning Tree
nav_order: 3
permalink: /tree/sending_root_announcements
---

# Sending Root Announcements

It is the responsibility of the root node to send a new root announcement on a regular interval (typically every **30 minutes**). To do this, it should create a new empty announcement, populating the **Root public key** field with the node’s own public key and the **Sequence number**.

Each node should maintain their own local sequence number, which is used only when sending out root announcements with their own key as the **Root public key**, ensuring that the sequence number increments only when a new announcement is generated (and not when repeating an update due to changes in the tree).

It is the responsibility of other non-root nodes to sign and repeat good root announcements received from their chosen parent to all directly peered nodes (including repeating the update to the chosen parent) at the time that it is received, although they must not attempt to modify the update in any other way (for example, by trying to modify the **Root public key**, **Root sequence** or any of the existing **Signature** fields).

Nodes must not repeat bad root announcements or any root announcements arriving from other peers that are not the chosen parent.
