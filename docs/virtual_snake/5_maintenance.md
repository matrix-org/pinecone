---
title: Routine Maintenance
parent: Virtual Snake
nav_order: 5
permalink: /virtual_snake/maintenance
---

# Routine Maintenance

At a specified interval, typically every 1 second, the node should run the following checks:

1. If the descending node entry has expired, that is, the time since the **Last seen** entry has passed 10 seconds, remove the entry;
2. If the descending node entry has different root information, remove the entry;
3. Remove any routing table entries that are older than 10 seconds.
