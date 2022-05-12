[![Docs](https://img.shields.io/badge/docs-main-blue.svg?style=flat-square)](https://matrix-org.github.io/pinecone/)

# Pinecone

Pinecone is an experimental overlay routing protocol suite which is the foundation of the current P2P Matrix demos. It is designed to provide end-to-end encrypted connectivity between devices at a global scale over any compatible medium (currently TCP, WebSockets, Bluetooth Low Energy etc), allowing multi-hop peer-to-peer connectivity between devices even in places where there is no Internet connectivity.

Pinecone builds two virtual topologies: a globally agreed spanning tree, like [Yggdrasil](https://github.com/yggdrasil-network/yggdrasil-go), and a virtual line (or snake) arranged sequentially by public key. This gives some rise to the routing scheme perhaps being called SNEK (Sequentially Networked Edwards Key) routing, but perhaps we can come up with a better acronym. 🐍

Intersecting paths between keyspace neighbours provide the bulk of the routing knowledge, whilst the spanning tree provides greedy routing for some bootstrap and path setup traffic. In addition, Pinecone also implements source routing and an active pathfinder, although these are currently not used by the P2P Matrix demos.

Pinecone is incredibly experimental still. There might be bugs, vulnerabilities or architectural problems that we don't yet know about. If you spot anything that doesn't look right, we are very happy to welcome issues and pull requests, or you can join us in [#p2p:matrix.org](https://matrix.to/#/#p2p:matrix.org).

## Requirements

Go 1.14 or later.

## Questions

### Is there any documentation?

The best documentation today is the code itself, which is reasonably well commented. There is also some documentation on how the protocol works in the [Pinecone Wiki on GitHub](https://github.com/matrix-org/pinecone/wiki). 

### Does Pinecone perform well?

Generally yes! However, this implementation isn't terribly well optimised yet so there will almost certainly be improvements that can be made.

### Will Pinecone scale?

It's a primary goal of ours to build something that will scale up to Internet-like proportions in order to help us with our plans for Matrix world domination. We think Pinecone should scale well, but ultimately we will find out and make changes accordingly.

### Is Pinecone secure?

All traffic transported over Pinecone is end-to-end encrypted using TLS, therefore intermediate nodes will not be able to inspect packet contents. Many of the protocol messages are also cryptographically signed for authenticity. The protocol is still in its infancy, however, so there may be theoretical attacks that we don't know about yet.

### Can I run Pinecone on my platform?

This implementation is written in [Go](https://golang.org) which has excellent support for a number of platforms and cross-compilation for mobile devices. We've successfully seen Pinecone running on macOS, Linux, Windows, Android and iOS. We aren't aware of any specific reasons that it wouldn't work on other platforms supported by Go.

### Why not Yggdrasil?

We did in fact experiment with Yggdrasil in earlier P2P Matrix demos, and in many ways, Pinecone is directly inspired by Yggdrasil. However, the spanning tree topology alone is not a suitable routing scheme for highly dynamic networks. Peerings that represent parent-child relationships on the spanning tree can result in entire parts of the coordinate system becoming temporarily invalidated, interrupting connectivity.

### Why not libp2p?

We experimented with that too! libp2p worked well for local-only scenarios but currently assumes in many places that nodes will be directly routable to each other over (either over a LAN or the Internet), which is not necessarily going to be the case for P2P Matrix. Other components that would be useful (such as NAT traversal and overlay routing) are still quite early. If Pinecone overlay routing is a success then we will hopefully to be able to assist with adding support into libp2p in the future.

### Does Pinecone provide anonymity?

No, it is not a goal of Pinecone to provide anonymity. Pinecone packets will be routed using the most direct paths possible (in contrast to Tor and friends, which deliberately send traffic well out of their way) and Pinecone packets do contain source and destination information in their headers currently. It is likely that we will be able to seal some of this information, in particular the source addresses, to reduce traffic correlation, but this is not done today.

### Does Pinecone work on mobile devices?

Yes, we actually have two P2P Matrix demos for Android and iOS today. Node mobility is an important problem for us to solve, as not many routing schemes today respond well to topology changes. We believe that the SNEK routing within Pinecone should respond well to topology changes.

### What is a static peer?

Pinecone nodes can automatically discover each other depending on the platform (for example, Android and iOS nodes can discover each other over Bluetooth without any further configuration). However, you might not be physically close to any other Pinecone nodes, so instead you can join the wider Pinecone network by connecting to a static peer over the Internet or an existing network. Traffic to distant nodes will be routed over this peer connection.

### Can I run my own static peer?

Sure, the `cmd/pinecone` binary will help you to do that. You will need to provide the `-listen` command line argument to specify which port to accept connections on, and you will probably also want to specify the `-connect` flag to connect your node to an existing peer so that your node is not isolated from the rest of the world. Unless, of course, isolation is what you are aiming for.

### Does Pinecone work through firewalls or NATs?

Yes. Pinecone peering connections look like regular TCP or WebSocket connections and will work fine through firewalls or NATs. If you make an outbound connection to a static node, you will still be able to receive incoming Pinecone traffic over that peering.
