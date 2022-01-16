import { addSubmitButton, nodeTypeToOptions, convertNodeTypeToID, nodeOptionsIndex } from "./common.js";
import { graph } from "../graph.js";
import { APICommandID, APICommandMessageID, SendToServer } from "../server-api.js";

let nodesFormNodeCount = 0;

function setupFormSubmission() {
    let addNodesForm = document.getElementById("add-nodes-form");
    if (addNodesForm) {
        addNodesForm.onsubmit = submitAddNodesForm;
    }
}
setupFormSubmission();

export function resetNodesFormCount() {
    nodesFormNodeCount = 0;
}

export function submitAddNodesForm(form) {
    let exists = false;
    let commands = [];
    let submitForm = true;
    let processedNodes = [];
    let nodeName = "";
    let nodeType = "";

    for (let z = 0; z < nodesFormNodeCount; z++) {
        let newNode = document.getElementById("new-node-" + z);
        let node = newNode.querySelector(".node-options");
        let options = node.querySelectorAll(".node-option");
        let nodeOptions = new Map();
        options.forEach(function(item) {
            nodeOptions.set(item.name, item.value);
        });

        let nodeNameEle = newNode.querySelector("input[name='nodename']");
        let nodeTypeEle = newNode.querySelector("select[name='nodetype']");

        if (nodeNameEle.value === "") {
            if (!nodeNameEle.className.includes(" focus-error")) {
                nodeNameEle.className += " focus-error";
            }
            // TODO : form feedback about empty node name/s
            submitForm = false;
        } else if (graph.nodeIDs.includes(nodeNameEle.value)) {
            if (!nodeNameEle.className.includes(" focus-error")) {
                nodeNameEle.className += " focus-error";
            }
            // TODO : form feedback about which node/s already exist
            exists = true;
            submitForm = false;
        } else if (processedNodes.includes(nodeNameEle.value)) {
            if (!nodeNameEle.className.includes(" focus-error")) {
                nodeNameEle.className += " focus-error";
            }
            // TODO : form feedback about duplicate nodes
            submitForm = false;
        } else {
            nodeNameEle.className = nodeNameEle.className.replace(" focus-error", "");
            nodeName = nodeNameEle.value;
        }

        processedNodes.push(nodeNameEle.value);
        nodeType = nodeTypeEle.value;

        if (nodeName != "" && nodeType != "") {
            let nodeID = convertNodeTypeToID(nodeType);
            commands.push({"MsgID": APICommandID.AddNode, "Event": {"Name": nodeName, "NodeType": nodeID}});
            switch(nodeType) {
            case "GeneralAdversary":
                commands.push({"MsgID": APICommandID.ConfigureAdversaryDefaults, "Event": {
                    "Node": nodeName,
                    "DropRates": Object.fromEntries(nodeOptions)
                }});
                break;
            }
        } else {
            console.log("Error parsing add nodes form.");
            submitForm = false;
        }
    }

    if (submitForm) {
        if (commands.length > 0) {
            SendToServer({"MsgID": APICommandMessageID.PlaySequence, "Events": commands});
        }

        let nodesModal = document.getElementById("add-nodes-modal");
        if (!nodesModal) {
            return;
        }

        nodesModal.style.display = "none";
    }
}

export function extendNodesForm() {
    let newNodeID = "new-node-" + nodesFormNodeCount;
    nodesFormNodeCount++;

    let nodesForm = document.getElementById("add-nodes-form");
    // Remove the submit button from the bottom
    nodesForm.removeChild(nodesForm.lastChild);

    let nodeTable = document.createElement('div');
    nodeTable.id = newNodeID;
    let hRule = document.createElement("hr");
    nodeTable.appendChild(hRule);
    let nodeName = document.createElement("div");
    nodeName.className = "row";
    let nodeNameCol = document.createElement("div");
    nodeNameCol.className = "col-two-left";
    let nodeNameLabel = document.createElement("label");
    nodeNameLabel.innerHTML = "Name";
    let nodeNameColTwo = document.createElement("div");
    nodeNameColTwo.className = "col-two-right";
    let nodeNameInput = document.createElement("input");
    nodeNameInput.type = "text";
    nodeNameInput.name = "nodename";
    nodeNameInput.placeholder = "Node name...";

    nodeNameCol.appendChild(nodeNameLabel);
    nodeNameColTwo.appendChild(nodeNameInput);
    nodeName.appendChild(nodeNameCol);
    nodeName.appendChild(nodeNameColTwo);
    nodeTable.appendChild(nodeName);

    let nodeType = document.createElement("div");
    nodeType.className = "row";
    let nodeCol = document.createElement("div");
    nodeCol.className = "col-two-left";
    let nodeLabel = document.createElement("label");
    nodeLabel.innerHTML = "Node Type";
    let nodeColTwo = document.createElement("div");
    nodeColTwo.className = "col-two-right";

    let typeSelect = document.createElement('select');
    typeSelect.name = "nodetype";
    typeSelect.onchange = e => {
        let node = document.getElementById(newNodeID);
        for (let i = 0; i < node.childNodes.length; i++) {
            if (node.childNodes[i].className.includes("node-options")) {
                node.removeChild(node.childNodes[i]);
                break;
            }
        }

        let options = nodeTypeToOptions.get(e.target.value)();
        node.appendChild(options);
    };

    let options = Array.from(nodeTypeToOptions.keys());
    for (let i = 0; i < options.length; i++) {
        let option = document.createElement("option");
        option.value = options[i];
        option.text = options[i];
        typeSelect.appendChild(option);
    }

    nodeCol.appendChild(nodeLabel);
    nodeColTwo.appendChild(typeSelect);
    nodeType.appendChild(nodeCol);
    nodeType.appendChild(nodeColTwo);
    nodeTable.append(nodeType);

    let nodeOptions = nodeTypeToOptions.get(nodeTypeToOptions.keys().next().value)();
    nodeTable.appendChild(nodeOptions);

    nodesForm.appendChild(nodeTable);
    addSubmitButton(nodesForm);

    let extendButton = document.getElementById("extend-nodes");
    extendButton.onclick = extendNodesForm;
}
