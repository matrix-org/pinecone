import { graph } from "./graph.js";
import { APICommandMessageID, APICommandID, SendToServer } from "./server-api.js";
import { resetPeeringsFormCount, extendPeeringsForm } from "./ui/peerings-form.js";
import { resetNodesFormCount, extendNodesForm } from "./ui/nodes-form.js";
import { addSubmitButton, nodeTypeToOptions, convertNodeTypeToID } from "./ui/common.js";

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

export function ResetReplayUI(element) {
    element.className = element.className.replace(" active", "");
    let tooltip = element.getElementsByClassName("tooltiptext")[0];
    tooltip.textContent = "Pause Events";
    let icon = element.getElementsByClassName("fa")[0];
    icon.className = icon.className.replace(" fa-play", " fa-pause");
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

    let anchor = "";
    switch(this.id) {
    case "peerTopo":
        anchor = "peer";
        break;
    case "snakeTopo":
        anchor = "snake";
        break;
    case "treeTopo":
        anchor = "tree";
        break;
    case "geographicTopo":
        anchor = "geographic";
        break;
    default:
        return;
    }

    graph.changeDataSet(anchor);
    window.location.href = "#" + anchor;
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
    case "ping-start-stop":
        handleToolPingStartStop(this);
        break;
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

export function SetPingToolState(active) {
    let subtool = document.getElementById("ping-start-stop");

    if (!active && subtool.className.includes("active")) {
        subtool.className = subtool.className.replace(" active", "");
        let tooltip = subtool.getElementsByClassName("tooltiptext")[0];
        tooltip.textContent = "Start Pings";

        if (subtool.className.includes("sub-active")) {
            subtool.className = subtool.className.replace(" sub-active", "");
        }
    } else if (active && !subtool.className.includes("active")) {
        subtool.className += " active";
        let tooltip = subtool.getElementsByClassName("tooltiptext")[0];
        tooltip.textContent = "Stop Pings";
    }
}

function handleToolPingStartStop(subtool) {
    let command = {"MsgID": APICommandID.Unknown, "Event": {}};
    if (subtool.className.includes("active")) {
        command.MsgID = APICommandID.StopPings;
    } else {
        command.MsgID = APICommandID.StartPings;
    }

    SendToServer({"MsgID": APICommandMessageID.PlaySequence, "Events": [command]});
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
        validSimCommands.set("StartPings", []);
        validSimCommands.set("StopPings", []);

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
    resetNodesFormCount();

    let nodesForm = document.getElementById("add-nodes-form");
    nodesForm.innerHTML = '<span><button id="extend-nodes" type="button" class="toggle extend-form-button">+</button></span><h4>New Nodes</h4>';

    addSubmitButton(nodesForm);
    extendNodesForm();

    let extendNodesButton = document.getElementById("extend-nodes");
    extendNodesButton.onclick = extendNodesForm;
}

function handleToolAddPeerings(subtool) {
    setupBaseModal("add-peerings-modal");
    resetPeeringsFormCount();

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
    case "StartPings":
        id = APICommandID.StartPings;
    case "StopPings":
        id = APICommandID.StopPings;
    default:
        break;
    }

    return id;
}
