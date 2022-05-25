---
title: Handling Root Announcements
parent: Spanning Tree
nav_order: 4
permalink: /tree/handling_root_announcements
---

# Handling Root Announcements

When a node receives a root announcement from a direct peer, it should do the following:

1. Perform sanity checks on the update;
2. Store the received update against the peer in memory;
3. Decide whether to re-parent;

If a root announcement is received from any peer during the 1 second reparent wait interval, it should be sanity-checked (step 1) and stored (step 2) but should not be processed any further. This is to ensure that many updates arriving quickly from direct peers will not result in a flood of root announcements being sent from the node in rapid succession, which overall reduces the load on the network and prevents the tree rebuilding excessively.

## Sanity checks

The following sanity checks must be performed on all incoming root announcement updates:

1. The update must be well-formed, with no missing fields or unexpected values;
2. There must be at least one signature;
3. The first **Signing public key** must match the **Root public key**;
4. The last **Signing public key** must match the public key of your direct peer;
5. None of the signature entries should have a **Destination port number** of 0;
6. The update must not contain any loops, that is that the update must not contain the same public key in the signatures more than once;
7. If a root update has been received from this peer before, and the **Root public key** is equal to the last update, the **Root sequence** must be greater than or equal to the last update.

If any of these conditions fail, the update is considered to be invalid, the update should be dropped and the peer that sent us this announcement should be disconnected as a result of the error.

## Storing root announcements

When storing a root announcement for a given peer, the following information should be kept:

- The announcement itself;
- The time the announcement was received;
- The order in which the announcement was received;
  - The order of received root announcements must be global across all peers;

## Deciding to re-parent

Once the sanity checks have passed, if the update came from the currently selected parent, perform the following checks in order:

1. If the reparent wait timer is active, do nothing and stop processing;
2. If any of the **Signatures** entries in the update contain the node’s own public key, implying that our parent has suddenly decided to choose us as their parent instead:
    1. Become a root node, sending an announcement to all of your direct peers notifying them of this fact;
    2. Start a 1 second reparent wait timer, after which the parent selection algorithm will run;
3. If the **Root public key** is lower than the last root announcement from our chosen parent, implying that either the previous root node has disappeared or the parent is notifying us of bad news:
    1. Become a root node, sending an announcement to all of your direct peers notifying them of this fact;
    2. Start a 1 second reparent wait timer, after which the parent selection algorithm will run;
4. If the **Root public key** and the **Root sequence** fields are both equal to those in the last root announcement from our chosen parent, implying that the parent is notifying us of bad news:
    1. Become a root node, sending an announcement to all of your direct peers notifying them of this fact;
    2. Start a 1 second reparent wait timer, after which the parent selection algorithm will run;
5. If the **Root public key** is higher than the last root announcement from our chosen parent, implying that our parent has learned about a stronger root:
    1. Send a tree announcement with the new update to all directly connected peers;
6. If the **Root public key** is equal to the last root announcement but the **Root sequence** is higher than the last root announcement:
    1. Send a tree announcement with the new update to all directly connected peers;

Otherwise, if the update arrived from another peer that is not our chosen parent, perform the following checks in order:

1. If the reparent wait timer is active, do nothing and stop processing;
2. If any of the **Signatures** entries in the update contain the node’s own public key, implying that the peer has chosen us a parent, do nothing and stop processing;
3. If the **Root public key** is higher than the last root announcement from our chosen parent:
    1. Set the node’s chosen parent to this specific peer;
    2. Send a tree announcement with the new update to all directly connected peers;
4. If the **Root public key** is lower than the last root announcement from our chosen parent:
    1. Send a tree announcement back to this peer only with the last root announcement from our chosen parent;
5. In any other case not matched by the above:
    1. Run the parent selection algorithm.
