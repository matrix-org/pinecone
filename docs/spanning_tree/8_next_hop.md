---
title: Next Hop Calculation
parent: Spanning Tree
nav_order: 8
permalink: /tree/next_hop
---

# Next Hop Calculation

When using tree routing to route towards a certain set of coordinates, a simple distance formula is used. To calculate the distance between coordinates A and B:

1. Calculate the length of the common prefix of both sets of coordinates;
2. Add the length of coordinates A to the length of coordinates B;
3. Subtract the length of the common prefix multiplied by 2.

As an illustrative example, where A is `[1 3 5 3 4]` and B is `[1 3 5 7 6 1]`:

1. Both A and B start with `[1 3 5]`, therefore the common prefix length is 3;
2. A has a length of 5, B has a length of 6;
3. (5 ＋ 6) － (3 ✕ 2) results in a total distance of 5 between A and B.

The next-hop selection for tree routing is as follows:

1. Start with the distance from our own coordinates to the destination coordinates as the <span style="text-decoration:underline;">best distance</span>, and the <span style="text-decoration:underline;">best candidate</span> as an empty field;
2. If the best distance is already 0 then the frame has arrived at its intended destination coordinates. Pass the frame to the local router and stop here, otherwise;
3. Iterate through all directly connected peers, checking the following conditions in order:
    1. Skip the peer and move onto the next if:
        1. The peer has not sent us a root announcement yet;
        2. The peer is the peer that sent us the frame that we are trying to forward (we will never route the frame back to where it came from as this would create a routing loop);
        3. The **Root public key** of the peer’s last root announcement does not match that of our chosen parent’s last announcement;
        4. The **Root sequence** of the peer’s last root announcement does not match that of our chosen parent’s last announcement;
    2. Calculate the distance from the peer’s coordinates to the destination coordinates;
    3. If the calculated distance is less than the best distance, update the best distance and mark the peer as the best candidate;
    4. Skip the peer and move onto the next if the calculated distance is greater than the best distance;
    5. If the best candidate is not empty and the peer’s last root announcement was received sooner than that of the best candidate, mark the peer as the best candidate.

If the best candidate is still empty at the end of the algorithm, there is no suitable next-hop candidate that will take the frame closer to its destination. A node must never route a tree-routed packet to a peer that will take it further away from the destination.
