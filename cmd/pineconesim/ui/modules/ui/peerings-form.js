import { addSubmitButton, createNodeOptionsDefault, convertTypeIDToNodeType, nodeTypeToOptions, nodeOptionsIndex } from "./common.js";
import { graph } from "../graph.js";
import { APICommandID, APICommandMessageID, SendToServer } from "../server-api.js";

let peeringsFormPeeringCount = 0;
let overrideOptionsIndex = 1;

function setupFormSubmission() {
    let addPeeringsForm = document.getElementById("add-peerings-form");
    if (addPeeringsForm) {
        addPeeringsForm.onsubmit = submitAddPeeringsForm;
    }
}
setupFormSubmission();

export function resetPeeringsFormCount() {
    peeringsFormPeeringCount = 0;
}

export function submitAddPeeringsForm(form) {
    let commands = [];
    for (let z = 0; z < peeringsFormPeeringCount; z++) {
        let newPeering = document.getElementById("new-peering-" + z);
        let node1 = newPeering.querySelector("select[name=node1]");
        let node2 = newPeering.querySelector("select[name=node2]");
        let nodes = newPeering.querySelectorAll(".node-options");

        if (nodes.length != 2) {
            console.warn("Form submitted without nodes selected.");
            return;
        }

        if (node1.value === node2.value) {
            if (!node2.className.includes(" focus-error")) {
                node2.className += " focus-error";
            }
            // TODO : provide feedback with failure
            return;
        } else {
            node2.className = node2.className.replace(" focus-error", "");
        }

        let nodeMap = new Map();

        for (let i = 0; i < 2; i++) {
            let options = nodes[i].querySelectorAll(".node-option");
            let nodeOptions = new Map();
            options.forEach(function(item) {
                nodeOptions.set(item.name, item.value);
            });

            let thisNode = null;
            let otherNode = null;
            if (i === 0) {
                thisNode = node1;
                otherNode = node2;
            } else {
                thisNode = node2;
                otherNode = node1;
            }

            if (nodeOptions.size > 0) {
                switch(convertTypeIDToNodeType(graph.getNodeType(thisNode.value))) {
                case "GeneralAdversary":
                    commands.push({"MsgID": APICommandID.ConfigureAdversaryPeer, "Event": {
                        "Node": thisNode.value,
                        "Peer": otherNode.value,
                        "DropRates": Object.fromEntries(nodeOptions)
                    }});
                    break;
                }
            }
        }

        commands.push({"MsgID": APICommandID.AddPeer, "Event": {"Node": node1.value, "Peer": node2.value}});
    }

    if (commands.length > 0) {
        SendToServer({"MsgID": APICommandMessageID.PlaySequence, "Events": commands});
    }

    let peeringsModal = document.getElementById("add-peerings-modal");
    if (!peeringsModal) {
        return;
    }

    peeringsModal.style.display = "none";
}

export function extendPeeringsForm() {
    let newPeeringID = "new-peering-" + peeringsFormPeeringCount;
    peeringsFormPeeringCount++;

    let peeringsForm = document.getElementById("add-peerings-form");
    // Remove the submit button from the bottom
    peeringsForm.removeChild(peeringsForm.lastChild);

    let peeringTable = document.createElement('div');
    peeringTable.id = newPeeringID;

    let hRule = document.createElement("hr");
    peeringTable.appendChild(hRule);
    let firstNode = document.createElement("div");
    firstNode.className = "row";
    let firstNodeCol = document.createElement("div");
    firstNodeCol.className = "col-two-left";
    let firstNodeLabel = document.createElement("label");
    firstNodeLabel.innerHTML = "<b><u>Node 1";
    let firstNodeColTwo = document.createElement("div");
    firstNodeColTwo.className = "col-two-right";
    let secondNode = document.createElement("div");
    secondNode.className = "row";
    let secondNodeCol = document.createElement("div");
    secondNodeCol.className = "col-two-left";
    let secondNodeLabel = document.createElement("label");
    secondNodeLabel.innerHTML = "<b><u>Node 2";
    let secondNodeColTwo = document.createElement("div");
    secondNodeColTwo.className = "col-two-right";

    let firstNodeOptions = document.createElement("div");
    firstNodeOptions.className = "row";
    let availablePeers1 = document.createElement('select');
    availablePeers1.className = "node-select";
    availablePeers1.name = "node1";
    availablePeers1.onchange = e => {
        updatePeeringNodeOptions(e.target.value, newPeeringID, "first-node");
    };

    let secondNodeOptions = document.createElement("div");
    secondNodeOptions.className = "row";
    let availablePeers2 = document.createElement('select');
    availablePeers2.className = "node-select";
    availablePeers2.name = "node2";
    availablePeers2.onchange = e => {
        updatePeeringNodeOptions(e.target.value, newPeeringID, "second-node");
    };

    for (let i = 0; i < graph.nodeIDs.length; i++) {
        let option = document.createElement("option");
        let optionDup = document.createElement("option");
        option.value = graph.nodeIDs[i];
        optionDup.value = graph.nodeIDs[i];
        option.text = graph.nodeIDs[i];
        optionDup.text = graph.nodeIDs[i];
        availablePeers1.appendChild(option);
        availablePeers2.appendChild(optionDup);
    }

    let options1 = null;
    let options2 = null;
    if (graph.nodeIDs.length > 0) {
        let nodeType = convertTypeIDToNodeType(graph.getNodeType(graph.nodeIDs[0]));
        options1 = nodeTypeToOptions.get(nodeType)();
        options2 = nodeTypeToOptions.get(nodeType)();
    }

    firstNodeCol.appendChild(firstNodeLabel);
    firstNodeColTwo.appendChild(availablePeers1);
    firstNode.appendChild(firstNodeCol);
    firstNode.appendChild(firstNodeColTwo);
    let firstDiv = document.createElement("div");
    firstDiv.name = "first-node";
    firstDiv.appendChild(firstNode);
    let firstCheckRow = document.createElement("div");
    firstCheckRow.className = "row";

    if (options1.childElementCount > 0) {
        let checkbox = createCheckbox();
        firstCheckRow.appendChild(checkbox);
    }
    firstDiv.appendChild(firstCheckRow);

    options1 = createNodeOptionsDefault();
    if (options1) {
        firstNodeOptions.appendChild(options1);
        firstDiv.appendChild(firstNodeOptions);
    }
    peeringTable.appendChild(firstDiv);

    secondNodeCol.appendChild(secondNodeLabel);
    secondNodeColTwo.appendChild(availablePeers2);
    secondNode.appendChild(secondNodeCol);
    secondNode.appendChild(secondNodeColTwo);
    let secondDiv = document.createElement("div");
    secondDiv.name = "second-node";
    secondDiv.appendChild(secondNode);
    let secondCheckRow = document.createElement("div");
    secondCheckRow.className = "row";

    if (options2.childElementCount > 0) {
        let checkbox = createCheckbox();
        secondCheckRow.appendChild(checkbox);
    }
    secondDiv.appendChild(secondCheckRow);

    options2 = createNodeOptionsDefault();
    if (options2) {
        secondNodeOptions.appendChild(options2);
        secondDiv.appendChild(secondNodeOptions);
    }
    peeringTable.appendChild(secondDiv);

    peeringsForm.appendChild(peeringTable);
    addSubmitButton(peeringsForm);

    let extendButton = document.getElementById("extend-peerings");
    extendButton.onclick = extendPeeringsForm;
}

function updatePeeringNodeOptions(nodeID, formSubID, elementName) {
    let node = document.getElementById(formSubID);
    for (let i = 0; i < node.childNodes.length; i++) {
        if (node.childNodes[i].name === elementName) {
            let nodeOptions = node.childNodes[i].childNodes[nodeOptionsIndex];
            for (let j = 0; j < nodeOptions.childNodes.length; j++) {
                if (nodeOptions.childNodes[j].className.includes("node-options")) {
                    nodeOptions.removeChild(nodeOptions.childNodes[j]);
                    break;
                }
            }

            let overrideOptions = node.childNodes[i].childNodes[overrideOptionsIndex];
            for (let j = 0; j < overrideOptions.childNodes.length; j++) {
                if (overrideOptions.childNodes[j].className.includes("override-options")) {
                    overrideOptions.removeChild(overrideOptions.childNodes[j]);
                    break;
                }
            }

            let nodeType = graph.getNodeType(nodeID);
            let options = nodeTypeToOptions.get(convertTypeIDToNodeType(nodeType))();

            if (options.childElementCount > 0) {
                let checkbox = createCheckbox();
                overrideOptions.appendChild(checkbox);
            }

            options = createNodeOptionsDefault();
            nodeOptions.appendChild(options);
            break;
        }
    }
}

function createCheckbox() {
    let box = document.createElement("div");
    box.className = "override-options";
    let expandOptions = document.createElement("label");
    expandOptions.className = "options-container";
    expandOptions.innerHTML = "Override Node Settings";
    let expandBox = document.createElement("input");
    expandBox.type = "checkbox";
    expandBox.checked = false;
    expandBox.onclick = e => {
        let options = createNodeOptionsDefault();
        let nodeForm = box.parentNode.parentNode;
        let nodeOptions = nodeForm.childNodes[nodeOptionsIndex];
        let nodeIDSelect = nodeForm.querySelector("select[class='node-select']");
        if (expandBox.checked) {
            let nodeType = graph.getNodeType(nodeIDSelect.value);
            options = nodeTypeToOptions.get(convertTypeIDToNodeType(nodeType))();
        }
        nodeOptions.removeChild(nodeOptions.childNodes[0]);
        nodeOptions.appendChild(options);
    };
    let expandMark = document.createElement("span");
    expandMark.className = "checkmark";
    expandOptions.appendChild(expandBox);
    expandOptions.appendChild(expandMark);
    box.appendChild(expandOptions);

    return box;
}
