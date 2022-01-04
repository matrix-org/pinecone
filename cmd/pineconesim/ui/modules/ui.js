import { graph } from "./graph.js";
import { APICommandMessageID, APICommandID, SendToServer } from "./server-api.js";

let leftShown = false;
let rightShown = false;

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
                if (content.hasOwnProperty("EventSequence")) {
                    let validSimCommands = new Map();
                    validSimCommands.set("Debug", []);
                    validSimCommands.set("Play", []);
                    validSimCommands.set("Pause", []);
                    validSimCommands.set("Delay", ["Length"]);
                    validSimCommands.set("AddNode", ["Name"]);
                    validSimCommands.set("RemoveNode", ["Name"]);
                    validSimCommands.set("AddPeer", ["Node", "Peer"]);
                    validSimCommands.set("RemovePeer", ["Node", "Peer"]);

                    let msgs = [];

                    for (let i = 0; i < content.EventSequence.length; i++) {
                        let sequence = content.EventSequence;
                        if (sequence[i].hasOwnProperty("Command") && validSimCommands.has(sequence[i].Command)) {
                            let command = sequence[i].Command;
                            let requiredDataFields = validSimCommands.get(command);
                            for (let j = 0; j < requiredDataFields.length; j++) {
                                if (sequence[i].hasOwnProperty("Data") && sequence[i].Data.hasOwnProperty(requiredDataFields[j])) {
                                    let id = convertCommandToID(command);
                                    msgs.push({"MsgID": id, "Event": sequence[i].Data});
                                } else {
                                    // TODO : Handle invalid sim command field
                                    console.log("Import error: " + JSON.stringify(sequence[i]) + " does not contain \"Data\" field \"" + requiredDataFields[j] + "\"");
                                }
                            }
                        } else {
                            // TODO : Handle invalid sim command
                            console.log("Import error: " + JSON.stringify(sequence[i]) + " is not a valid sim command");
                        }
                    }

                    SendToServer({"MsgID": APICommandMessageID.PlaySequence, "Events": msgs});
                }
                else {
                    // TODO : Handle json not having right fields
                    console.log("Import error: JSON does not include EventSequence field");
                }
            }
        };
    };

    input.click();
}

function handleToolReplayPlayPause(subtool) {
    let command = {"MsgID": APICommandID.Unknown, "Event": {}};
    if (subtool.className.includes("active")) {
        ResetReplayUI(subtool);

        command.MsgID = APICommandID.Pause;
    } else {
        subtool.className += " active";
        let tooltip = subtool.getElementsByClassName("tooltiptext")[0];
        tooltip.textContent = "Pause Replay";
        let icon = subtool.getElementsByClassName("fa")[0];
        icon.className = icon.className.replace(" fa-repeat", " fa-pause");

        command.MsgID = APICommandID.Play;
    }

    SendToServer({"MsgID": APICommandMessageID.PlaySequence, "Events": [command]});
}

function handleToolAddNodes(subtool) {
    setupBaseModal("add-nodes-modal");

    let nodesForm = document.getElementById("add-nodes-form");
    nodesForm.innerHTML = '<span><button id="extend-nodes" type="button" class="toggle extend-form-button">+</button></span><h4>New Nodes</h4>';

    addSubmitButton(nodesForm);
    extendNodesForm();

    let extendNodesButton = document.getElementById("extend-nodes");
    extendNodesButton.onclick = extendNodesForm;
}

function handleToolAddPeerings(subtool) {
    setupBaseModal("add-peerings-modal");

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
    default:
        break;
    }

    return id;
}

export function ResetReplayUI(element) {
    element.className = element.className.replace(" active", "");
    let tooltip = element.getElementsByClassName("tooltiptext")[0];
    tooltip.textContent = "Resume Replay";
    let icon = element.getElementsByClassName("fa")[0];
    icon.className = icon.className.replace(" fa-pause", " fa-repeat");
}

function submitAddNodesForm(form) {
    let exists = false;
    let commands = [];
    let submitForm = true;
    let processedNodes = [];
    for (let i = 0; i < form.target.length; i++) {
        if (form.target[i].name === "nodename") {
            if (form.target[i].value === "") {
                if (!form.target[i].className.includes(" focus-error")) {
                    form.target[i].className += " focus-error";
                }
                // TODO : form feedback about empty node name/s
                submitForm = false;
            } else if (graph.nodeIDs.includes(form.target[i].value)) {
                if (!form.target[i].className.includes(" focus-error")) {
                    form.target[i].className += " focus-error";
                }
                // TODO : form feedback about which node/s already exist
                exists = true;
                submitForm = false;
            } else if (processedNodes.includes(form.target[i].value)) {
                if (!form.target[i].className.includes(" focus-error")) {
                    form.target[i].className += " focus-error";
                }
                // TODO : form feedback about duplicate nodes
                submitForm = false;
            } else {
                form.target[i].className = form.target[i].className.replace(" focus-error", "");
                commands.push({"MsgID": APICommandID.AddNode, "Event": {"Name": form.target[i].value}});
            }

            processedNodes.push(form.target[i].value);
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

function submitAddPeeringsForm(form) {
    let isTo = false;
    let fromNode, toNode;
    let commands = [];
    let submitForm = true;
    for (let i = 0; i < form.target.length; i++) {
        if (form.target[i].name === "peer") {
            if (isTo) {
                toNode = form.target[i].value;
                if (fromNode != toNode) {
                    form.target[i].className = form.target[i].className.replace(" focus-error", "");
                    commands.push({"MsgID": APICommandID.AddPeer, "Event": {"Node": fromNode, "Peer": toNode}});
                } else {
                    if (!form.target[i].className.includes(" focus-error")) {
                        form.target[i].className += " focus-error";
                    }
                    // TODO : provide feedback with failure
                    submitForm = false;
                }
            } else {
                fromNode = form.target[i].value;
            }

            isTo = !isTo;
        }
    }

    if (submitForm) {
        if (commands.length > 0) {
            SendToServer({"MsgID": APICommandMessageID.PlaySequence, "Events": commands});
        }

        let peeringsModal = document.getElementById("add-peerings-modal");
        if (!peeringsModal) {
            return;
        }

        peeringsModal.style.display = "none";
    }
}

function extendPeeringsForm() {
    let peeringsForm = document.getElementById("add-peerings-form");
    // Remove the submit button from the bottom
    peeringsForm.removeChild(peeringsForm.lastChild);
    let peeringTable = document.createElement('div');
    let availablePeers = '<select name="peer">';
    for (let i = 0; i < graph.nodeIDs.length; i++) {
        availablePeers += '<option value="' + graph.nodeIDs[i] + '">' + graph.nodeIDs[i] + '</option>';
    }
    availablePeers += '</select>';

    peeringTable.innerHTML += '<hr>' +
        '<div class="row">' +
        '<div class="col-25">' +
        '<label for="from">From</label>' +
        '</div>' +
        '<div class="col-75">' +
        availablePeers +
        '</div>' +
        '</div>' +
        '<div class="row">' +
        '<div class="col-25">' +
        '<label for="to">To</label>' +
        '</div>' +
        '<div class="col-75">' +
        availablePeers +
        '</div>' +
        '</div>';

    peeringsForm.appendChild(peeringTable);
    addSubmitButton(peeringsForm);

    let extendButton = document.getElementById("extend-peerings");
    extendButton.onclick = extendPeeringsForm;
}

function addSubmitButton(form) {
    let submitButton = document.createElement('div');
    submitButton.innerHTML = '<div class="row">' +
        '<input type="submit" value="Submit">' +
        '</div>';

    form.appendChild(submitButton);
}

function extendNodesForm() {
    let nodesForm = document.getElementById("add-nodes-form");
    // Remove the submit button from the bottom
    nodesForm.removeChild(nodesForm.lastChild);
    let peeringTable = document.createElement('div');
    peeringTable.innerHTML += '<hr>' +
        '<div class="row">' +
        '<div class="col-25">' +
        '<label for="node">Name</label>' +
        '</div>' +
        '<div class="col-75">' +
        '<input type="text" name="nodename" placeholder="Node name...">' +
        '</div>' +
        '</div>';

    nodesForm.appendChild(peeringTable);
    addSubmitButton(nodesForm);

    let extendButton = document.getElementById("extend-nodes");
    extendButton.onclick = extendNodesForm;
}
