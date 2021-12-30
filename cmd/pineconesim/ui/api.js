export const APIEventMessageID = {
    Unknown: 0,
    InitialState: 1,
    StateUpdate: 2,
};

export const APICommandMessageID = {
    Unknown: 0,
    PlaySequence: 1,
};

export const APIUpdateID = {
    Unknown: 0,
    NodeAdded: 1,
    NodeRemoved: 2,
    PeerAdded: 3,
    PeerRemoved: 4,
    TreeParentUpdated: 5,
    SnakeAscUpdated: 6,
    SnakeDescUpdated: 7,
    TreeRootAnnUpdated: 8,
};

export const APICommandID = {
    Unknown: 0,
    Debug: 1,
    Delay: 2,
    AddNode: 3,
    RemoveNode: 4,
    AddPeer: 5,
    RemovePeer: 6,
};
