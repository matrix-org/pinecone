import { graph } from "./graph.js";
import { APICommandMessageID, APICommandID, SendToServer } from "./server-api.js";

let leftShown = false;
let rightShown = false;

let nodesFormNodeCount = 0;
let peeringsFormPeeringCount = 0;

let nodeOptionsIndex = 2;
let overrideOptionsIndex = 1;

let nodeTypeToOptions = new Map();
nodeTypeToOptions.set("Default", createNodeOptionsDefault);
nodeTypeToOptions.set("GeneralAdversary", createNodeOptionsGeneralAdversary);

export function openRightPanel() {
    if (!rightShown) {
        toggleRightPanel();
    }
}

export function closeRightPanel() {
    if (rightShown) {
        toggleRightPanel();
    }
}

function toggleLeftPanel() {
    let panel = document.getElementById("left");

    if (panel) {
        if (!leftShown) {
            panel.style.width = "max-content";
            leftShown= true;
        } else {
            panel.style.width = "0";
            leftShown= false;
        }
    }
}
document.getElementById("leftToggle").onclick = toggleLeftPanel;

function toggleRightPanel() {
    let panel = document.getElementById("right");

    if (panel) {
        if (!rightShown) {
            panel.style.width = "max-content";
            let toggleSize = parseInt(getComputedStyle(document.documentElement)
                                      .getPropertyValue('--toggle-box-size'), 10);
            let toggleSizePadding = parseInt(getComputedStyle(document.documentElement)
                                             .getPropertyValue('--toggle-box-margin'), 10);

            panel.style.minWidth = (toggleSize + toggleSizePadding * 2).toString() + "px";
            rightShown= true;
        } else {
            panel.style.width = "0";
            panel.style.minWidth = "0vw";
            rightShown= false;
        }
    }
}
document.getElementById("rightToggle").onclick = toggleRightPanel;

function focusSelectedNode() {
    let button = document.getElementById("focusNode");
    if (button) {
        graph.focusSelectedNode();
    }
}
document.getElementById("focusNode").onclick = focusSelectedNode;

function selectNetworkType(networkType) {
    let selectionTabs = document.getElementsByClassName("netselect");
    for (let i = 0; i < selectionTabs.length; i++) {
        selectionTabs[i].className = selectionTabs[i].className.replace(" active", "");
    }

    this.className += " active";

    switch(this.id) {
    case "peerTopo":
        graph.changeDataSet("peer");
        break;
    case "snakeTopo":
        graph.changeDataSet("snake");
        break;
    case "treeTopo":
        graph.changeDataSet("tree");
        break;
    case "geographicTopo":
        graph.changeDataSet("geographic");
        break;
    }
}

function setupNetworkSelection() {
    let selectionTabs = document.getElementsByClassName("netselect");
    for (let i = 0; i < selectionTabs.length; i++) {
        selectionTabs[i].onclick = selectNetworkType;
    }
}
setupNetworkSelection();

function selectTool(toolType) {
    switch(this.id) {
    case "scenario-new":
        handleToolScenarioNew(this);
        break;
    case "scenario-load":
        handleToolScenarioLoad(this);
        break;
    case "capture-start-stop":
        handleToolCaptureStartStop(this);
        break;
    case "replay-upload":
        handleToolReplayUpload(this);
        break;
    case "replay-play-pause":
        handleToolReplayPlayPause(this);
        break;
    case "add-nodes":
        handleToolAddNodes(this);
        break;
    case "add-peerings":
        handleToolAddPeerings(this);
        break;
    case "remove":
        handleToolRemove(this);
        break;
    }
}

function handleToolScenarioNew(subtool) {
    // TODO
}

function handleToolScenarioLoad(subtool) {
    // TODO
}

function handleToolCaptureStartStop(subtool) {
    if (subtool.className.includes("active")) {
        subtool.className = subtool.className.replace(" active", "");
        let tooltip = subtool.getElementsByClassName("tooltiptext")[0];
        tooltip.textContent = "Start Capture";
        // TODO : stop capture
    } else {
        subtool.className += " active";
        let tooltip = subtool.getElementsByClassName("tooltiptext")[0];
        tooltip.textContent = "Stop Capture";
        // TODO : start capture
    }
}

function handleToolReplayUpload(subtool) {
    // Pop open a new file selection dialog
    let input = document.createElement('input');
    input.type = 'file';

    input.onchange = e => {
        let file = e.target.files[0];
        let reader = new FileReader();
        reader.readAsText(file,'UTF-8');

        reader.onload = readerEvent => {
            let content;
            try {
                content = JSON.parse(readerEvent.target.result);
            }
            catch(err) {
                // TODO : Handle json parse failure
                console.log(err);
            }

            if (content) {
                if (validateEventSequence(content)) {
                    let msgs = [];
                    for (let i = 0; i < content.EventSequence.length; i++) {
                        let id = convertCommandToID(content.EventSequence[i].Command);
                        if (id === APICommandID.AddNode) {
                            let nodeType = convertNodeTypeToID(content.EventSequence[i].Data.NodeType);
                            content.EventSequence[i].Data.NodeType = nodeType;
                        }
                        msgs.push({"MsgID": id, "Event": content.EventSequence[i].Data});
                    }

                    SendToServer({"MsgID": APICommandMessageID.PlaySequence, "Events": msgs});
                }
            }
        };
    };

    input.click();
}

function validateEventSequence(content) {
    let validSequence = false;
    if (content.hasOwnProperty("EventSequence")) {
        let validCommandCount = 0;
        let validSimCommands = new Map();
        validSimCommands.set("Debug", []);
        validSimCommands.set("Play", []);
        validSimCommands.set("Pause", []);
        validSimCommands.set("Delay", ["Length"]);
        validSimCommands.set("AddNode", ["Name", "NodeType"]);
        validSimCommands.set("RemoveNode", ["Name"]);
        validSimCommands.set("AddPeer", ["Node", "Peer"]);
        validSimCommands.set("RemovePeer", ["Node", "Peer"]);
        validSimCommands.set("ConfigureAdversaryDefaults", ["Node", "DropRates"]);
        validSimCommands.set("ConfigureAdversaryPeer", ["Node", "Peer", "DropRates"]);

        let validSubcommands = new Map();
        validSubcommands.set("DropRates", ["Overall", "Keepalive", "TreeAnnouncement", "TreeRouted", "VirtualSnakeBootstrap", "VirtualSnakeBootstrapACK", "VirtualSnakeSetup", "VirtualSnakeSetupACK", "VirtualSnakeTeardown", "VirtualSnakeRouted"]);

        for (let i = 0; i < content.EventSequence.length; i++) {
            let sequence = content.EventSequence;
            if (sequence[i].hasOwnProperty("Command") && validSimCommands.has(sequence[i].Command)) {
                let command = sequence[i].Command;
                let requiredDataFields = validSimCommands.get(command);
                let validFieldCount = 0;
                if (sequence[i].hasOwnProperty("Data")) {
                    for (let j = 0; j < requiredDataFields.length; j++) {
                        if (sequence[i].Data.hasOwnProperty(requiredDataFields[j])) {
                            if (validSubcommands.has(requiredDataFields[j])) {
                                let requiredSubcommands = validSubcommands.get(requiredDataFields[j]);
                                let cmdField = sequence[i].Data[requiredDataFields[j]];
                                let validSubcommandCount = 0;
                                for (let k = 0; k < requiredSubcommands.length; k++) {
                                    if (cmdField.hasOwnProperty(requiredSubcommands[k])) {
                                        if (validateField(requiredSubcommands[k], cmdField[requiredSubcommands[k]])) {
                                            validSubcommandCount++;
                                        } else {
                                            // TODO : Handle invalid sim command field
                                            console.log("Import error: " + JSON.stringify(sequence[i]) + " has an invalid value for \"" + requiredSubcommands[k] + "\"");
                                        }
                                    } else {
                                        // TODO : Handle invalid sim command field
                                        console.log("Import error: " + JSON.stringify(sequence[i]) + " does not contain required subcommand \"" + requiredSubcommands[k] + "\"");
                                    }
                                }

                                if (validSubcommandCount === requiredSubcommands.length && validSubcommandCount === Object.keys(cmdField).length) {
                                    validFieldCount++;
                                } else {
                                    // TODO : Handle invalid sim command field
                                    console.log("Import error: " + JSON.stringify(sequence[i]) + " has the wrong number of fields for " + requiredDataFields[j]);
                                }
                            } else {
                                let cmdField = sequence[i].Data[requiredDataFields[j]];
                                if (validateField(requiredDataFields[j], cmdField)) {
                                    validFieldCount++;
                                } else {
                                    // TODO : Handle invalid sim command field
                                    console.log("Import error: " + JSON.stringify(sequence[i]) + " has an invalid value for \"" + requiredDataFields[j] + "\"");
                                }
                            }
                        } else {
                            // TODO : Handle invalid sim command field
                            console.log("Import error: " + JSON.stringify(sequence[i]) + " does not contain required \"Data\" field \"" + requiredDataFields[j] + "\"");
                        }
                    }

                    // Check that there are only valid fields, and the right amount of fields
                    if (validFieldCount === requiredDataFields.length && validFieldCount === Object.keys(sequence[i].Data).length) {
                        validCommandCount++;
                    } else {
                        // TODO : Handle invalid sim command
                        console.log("Import error: " + JSON.stringify(sequence[i]) + " has the wrong number of Data fields");
                    }
                } else {
                    // TODO : Handle invalid sim command
                    console.log("Import error: " + JSON.stringify(sequence[i]) + " is missing the Data field");
                }
            } else {
                // TODO : Handle invalid sim command
                console.log("Import error: " + JSON.stringify(sequence[i]) + " is not a valid sim command");
            }
        }

        if (validCommandCount === content.EventSequence.length) {
            validSequence = true;
        }
    }
    else {
        // TODO : Handle json not having right fields
        console.log("Import error: JSON does not include EventSequence field");
    }

    return validSequence;
}

function validateField(field, value) {
    let validFieldValues = new Map();
    validFieldValues.set("NodeType", Array.from(nodeTypeToOptions.keys()));
    let isValid = false;

    if (validFieldValues.has(field)) {
        if (validFieldValues.get(field).includes(value)) {
            isValid = true;
        }
    } else {
        isValid = true;
    }

    return isValid;
}

function handleToolReplayPlayPause(subtool) {
    let command = {"MsgID": APICommandID.Unknown, "Event": {}};
    if (subtool.className.includes("active")) {
        ResetReplayUI(subtool);

        command.MsgID = APICommandID.Play;
    } else {
        subtool.className += " active";
        let tooltip = subtool.getElementsByClassName("tooltiptext")[0];
        tooltip.textContent = "Play Events";
        let icon = subtool.getElementsByClassName("fa")[0];
        icon.className = icon.className.replace(" fa-pause", " fa-play");

        command.MsgID = APICommandID.Pause;
    }

    SendToServer({"MsgID": APICommandMessageID.PlaySequence, "Events": [command]});
}

function handleToolAddNodes(subtool) {
    setupBaseModal("add-nodes-modal");
    nodesFormNodeCount = 0;

    let nodesForm = document.getElementById("add-nodes-form");
    nodesForm.innerHTML = '<span><button id="extend-nodes" type="button" class="toggle extend-form-button">+</button></span><h4>New Nodes</h4>';

    addSubmitButton(nodesForm);
    extendNodesForm();

    let extendNodesButton = document.getElementById("extend-nodes");
    extendNodesButton.onclick = extendNodesForm;
}

function handleToolAddPeerings(subtool) {
    setupBaseModal("add-peerings-modal");
    peeringsFormPeeringCount = 0;

    let peeringsForm = document.getElementById("add-peerings-form");
    peeringsForm.innerHTML = '<span><button id="extend-peerings" type="button" class="toggle extend-form-button">+</button></span><h4>New Peer Connections</h4>';

    addSubmitButton(peeringsForm);
    extendPeeringsForm();

    let extendPeeringsButton = document.getElementById("extend-peerings");
    extendPeeringsButton.onclick = extendPeeringsForm;
}

function handleToolRemove(subtool) {
    let commands = [];
    let nodes = graph.GetSelectedNodes();
    let peerings = graph.GetSelectedPeerings();

    if (nodes) {
        for (let i = 0; i < nodes.length; i++) {
            commands.push({"MsgID": APICommandID.RemoveNode, "Event": {"Name": nodes[i]}});
        }
    }

    if (peerings) {
        for (let i = 0; i < peerings.length; i++) {
            if (nodes && (nodes.includes(peerings[i].from) || nodes.includes(peerings[i].to))) {
                // Don't send redundant commands if a node is already being removed.
                continue;
            }
            commands.push({"MsgID": APICommandID.RemovePeer, "Event": {"Node": peerings[i].from, "Peer": peerings[i].to}});
        }
    }

    if (commands.length > 0) {
        SendToServer({"MsgID": APICommandMessageID.PlaySequence, "Events": commands});
    }
}

function setupBaseModal(modalName) {
    // Get the modal
    let modal = document.getElementById(modalName);
    if (!modal) {
        return;
    }

    // Get the <span> element that closes the modal
    let pClose = modal.getElementsByClassName("close")[0];

    modal.style.display = "block";

    // When the user clicks on <span> (x), close the modal
    if (pClose) {
        pClose.onclick = function() {
            modal.style.display = "none";
        };
    }

    // When the user clicks anywhere outside of the modal, close it
    window.onclick = function(event) {
        if (event.target == modal) {
            modal.style.display = "none";
        }
    };
}

function setupToolSelection() {
    let tools = document.getElementsByClassName("toolselect");
    for (let i = 0; i < tools.length; i++) {
        tools[i].onclick = selectTool;
    }

    let subtools = document.getElementsByClassName("subtoolselect");
    for (let i = 0; i < subtools.length; i++) {
        subtools[i].onclick = selectTool;
    }
}
setupToolSelection();

function setupFormSubmission() {
    let addPeeringsForm = document.getElementById("add-peerings-form");
    if (addPeeringsForm) {
        addPeeringsForm.onsubmit = submitAddPeeringsForm;
    }

    let addNodesForm = document.getElementById("add-nodes-form");
    if (addNodesForm) {
        addNodesForm.onsubmit = submitAddNodesForm;
    }
}
setupFormSubmission();

function convertNodeTypeToID(nodeType) {
    let typeID = 0;
    switch(nodeType) {
    case "Default":
        typeID = 1;
        break;
    case "GeneralAdversary":
        typeID = 2;
        break;
    }

    return typeID;
}

function convertTypeIDToNodeType(typeID) {
    let nodeType = "";
    switch(typeID) {
    case 1:
        nodeType = "Default";
        break;
    case 2:
        nodeType = "GeneralAdversary";
        break;
    }

    return nodeType;
}

function convertCommandToID(command) {
    let id = APICommandID.Unknown;
    switch (command) {
    case "Debug":
        id = APICommandID.Debug;
        break;
    case "Play":
        id = APICommandID.Play;
        break;
    case "Pause":
        id = APICommandID.Pause;
        break;
    case "Delay":
        id = APICommandID.Delay;
        break;
    case "AddNode":
        id = APICommandID.AddNode;
        break;
    case "RemoveNode":
        id = APICommandID.RemoveNode;
        break;
    case "AddPeer":
        id = APICommandID.AddPeer;
        break;
    case "RemovePeer":
        id = APICommandID.RemovePeer;
        break;
    case "ConfigureAdversaryDefaults":
        id = APICommandID.ConfigureAdversaryDefaults;
        break;
    case "ConfigureAdversaryPeer":
        id = APICommandID.ConfigureAdversaryPeer;
        break;
    default:
        break;
    }

    return id;
}

export function ResetReplayUI(element) {
    element.className = element.className.replace(" active", "");
    let tooltip = element.getElementsByClassName("tooltiptext")[0];
    tooltip.textContent = "Pause Events";
    let icon = element.getElementsByClassName("fa")[0];
    icon.className = icon.className.replace(" fa-play", " fa-pause");
}

function submitAddPeeringsForm(form) {
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

function extendPeeringsForm() {
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

    options1 = nodeTypeToOptions.get("Default")();
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

    options2 = nodeTypeToOptions.get("Default")();
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

            options = nodeTypeToOptions.get("Default")();
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
        let options = nodeTypeToOptions.get("Default")();
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

function addSubmitButton(form) {
    let submitButton = document.createElement('div');
    submitButton.innerHTML = '<div class="row">' +
        '<input type="submit" value="Submit">' +
        '</div>';

    form.appendChild(submitButton);
}

function submitAddNodesForm(form) {
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

function extendNodesForm() {
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

function createNodeOptionsDefault() {
    let nodeOptions = document.createElement("div");
    nodeOptions.className += " node-options";
    return nodeOptions;
}

function createNodeOptionsGeneralAdversary() {
    let nodeOptions = document.createElement("div");
    nodeOptions.className += " node-options";
    let drop = document.createElement("div");
    drop.className = "row";
    let dropLabel = document.createElement("label");
    dropLabel.innerHTML = "<b>Drop Rates:";
    drop.appendChild(dropLabel);
    nodeOptions.appendChild(drop);

    let advType = document.createElement("div");
    advType.className = "row";
    let advCol = document.createElement("div");
    advCol.className = "col-two-left";
    let advLabel = document.createElement("label");
    advLabel.innerHTML = "Template";
    let advColTwo = document.createElement("div");
    advColTwo.className = "col-two-right";

    let settingsMap = {
        "None": {
            "Overall": 0,
            "Keepalive": 0,
            "TreeAnnouncement": 0,
            "TreeRouted": 0,
            "VirtualSnakeBootstrap": 0,
            "VirtualSnakeBootstrapACK": 0,
            "VirtualSnakeSetup": 0,
            "VirtualSnakeSetupACK": 0,
            "VirtualSnakeTeardown": 0,
            "VirtualSnakeRouted": 0,
        },
        "BlockTreeProtoTraffic": {
            "Overall": 0,
            "Keepalive": 0,
            "TreeAnnouncement": 100,
            "TreeRouted": 0,
            "VirtualSnakeBootstrap": 0,
            "VirtualSnakeBootstrapACK": 0,
            "VirtualSnakeSetup": 0,
            "VirtualSnakeSetupACK": 0,
            "VirtualSnakeTeardown": 0,
            "VirtualSnakeRouted": 0,
        },
        "BlockSNEKProtoTraffic": {
            "Overall": 0,
            "Keepalive": 0,
            "TreeAnnouncement": 0,
            "TreeRouted": 0,
            "VirtualSnakeBootstrap": 100,
            "VirtualSnakeBootstrapACK": 100,
            "VirtualSnakeSetup": 100,
            "VirtualSnakeSetupACK": 100,
            "VirtualSnakeTeardown": 100,
            "VirtualSnakeRouted": 0,
        },
        "BlockOverlayTraffic": {
            "Keepalive": 0,
            "TreeAnnouncement": 0,
            "TreeRouted": 0,
            "VirtualSnakeBootstrap": 0,
            "VirtualSnakeBootstrapACK": 0,
            "VirtualSnakeSetup": 0,
            "VirtualSnakeSetupACK": 0,
            "VirtualSnakeTeardown": 0,
            "VirtualSnakeRouted": 100,
        },
    };

    let templateSelect = document.createElement('select');
    templateSelect.onchange = e => {
        let settings = settingsMap[e.target.value];
        for (const key in settings) {
            let slider = nodeOptions.querySelector("input[name='" + key + "']");
            slider.value = settings[key];

            let label = nodeOptions.querySelector("label[class='" + key + "-label']");
            label.innerHTML = settings[key] + "%";
        }
    };

    let templates = Array.from(Object.keys(settingsMap));
    for (let i = 0; i < templates.length; i++) {
        let template = document.createElement("option");
        template.value = templates[i];
        template.text = templates[i];
        templateSelect.appendChild(template);
    }

    advCol.appendChild(advLabel);
    advColTwo.appendChild(templateSelect);
    advType.appendChild(advCol);
    advType.appendChild(advColTwo);
    nodeOptions.append(advType);


    let allTraffic = generateSliderRow("Overall", "Overall");
    nodeOptions.appendChild(allTraffic);

    let keepalive = generateSliderRow("Keepalive", "Keepalive");
    nodeOptions.appendChild(keepalive);

    let tree1 = generateSliderRow("Tree Announcement", "TreeAnnouncement");
    nodeOptions.appendChild(tree1);
    let tree2 = generateSliderRow("Tree Routed", "TreeRouted");
    nodeOptions.appendChild(tree2);

    let snek1 = generateSliderRow("SNEK Bootstrap", "VirtualSnakeBootstrap");
    nodeOptions.appendChild(snek1);
    let snek3 = generateSliderRow("SNEK Bootstrap ACK", "VirtualSnakeBootstrapACK");
    nodeOptions.appendChild(snek3);

    let snek4 = generateSliderRow("SNEK Setup", "VirtualSnakeSetup");
    nodeOptions.appendChild(snek4);
    let snek5 = generateSliderRow("SNEK Setup ACK", "VirtualSnakeSetupACK");
    nodeOptions.appendChild(snek5);

    let snek6 = generateSliderRow("SNEK Teardown", "VirtualSnakeTeardown");
    nodeOptions.appendChild(snek6);

    let snek2 = generateSliderRow("SNEK Routed", "VirtualSnakeRouted");
    nodeOptions.appendChild(snek2);

    return nodeOptions;
}

function generateSliderRow(label, name) {
    let sliderDiv = document.createElement("div");
    sliderDiv.className = "row";
    let sliderCol = document.createElement("div");
    sliderCol.className = "col-two-left";
    let sliderLabel = document.createElement("label");
    sliderLabel.innerHTML = label;
    let sliderColTwo = document.createElement("div");
    sliderColTwo.className = "col-three-middle";
    let sliderColThree = document.createElement("div");
    sliderColThree.className = "col-three-right";
    let myLabel = document.createElement("label");
    myLabel.className = name + "-label";
    myLabel.style.position = "absolute";
    myLabel.style.marginRight = "6px";
    myLabel.style.right = "0";

    let sliderContainer = document.createElement("div");
    sliderContainer.className = "slidecontainer";
    let slider = document.createElement("input");
    slider.className = "slider node-option";
    slider.type = "range";
    slider.min = 0;
    slider.max = 100;
    slider.value = 0;
    slider.name = name;

    myLabel.innerHTML = slider.value + "%";
    let sliderLabelUpdate = function() {
        myLabel.innerHTML = this.value + "%";
    };
    slider.oninput = sliderLabelUpdate;
    sliderCol.appendChild(sliderLabel);
    sliderContainer.appendChild(slider);
    sliderColTwo.appendChild(sliderContainer);
    sliderColThree.appendChild(myLabel);
    sliderDiv.appendChild(sliderCol);
    sliderDiv.appendChild(sliderColTwo);
    sliderDiv.appendChild(sliderColThree);

    return sliderDiv;
}
