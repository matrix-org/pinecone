---
title: Routine Maintenance
parent: Virtual Snake
nav_order: 8
permalink: /virtual_snake/maintenance
---

# Routine Maintenance

At a specified interval, typically every 1 second, the node should run the following checks:

1. If the descending node entry has expired, that is, the time since the **Last seen** entry has passed 1 hour, tear down the path;
2. If the ascending node entry has expired, that is, the time since the **Last seen** entry has passed 1 hour, tear down the path;
3. If the ascending node entry is empty, either before or after step 2, send a bootstrap message into the network.
