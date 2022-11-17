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

export function updateRoutingTableChart() {
    let tableSizes = graph.getRoutingTableSizes();
    let routingTableSizes = Object.fromEntries(tableSizes);

    analyticsCharts['routing-table-size'].data.datasets[0].data = routingTableSizes;
    if (currentAnalyticsChart.name === 'routing-table-size') {
        graphDataUpdated = true;
    }
}

var graphDataUpdated = false;
setInterval(updateGraphs, 2000);

function updateGraphs() {
    if (graphDataUpdated) {
        currentAnalyticsChart.chart.update();
        graphDataUpdated = false;
    }
}

export function UpdateBandwidthGraphData(bwIntervalSeconds) {
    let distribution = graph.getNodeBandwidthDistribution();
    let labels = new Array();
    let protoDist = new Array();

    let conversionToKBPS = 8 / 1000 / bwIntervalSeconds;

    for (const [key, value] of [...distribution.entries()]) {
        labels.push(key * conversionToKBPS); // bytes to Kbps
        protoDist.push(value.Protocol);
    }

    let distributionGraph = 'bandwidth-distribution';
    analyticsCharts[distributionGraph].data.labels = labels;
    analyticsCharts[distributionGraph].data.datasets[0].data = protoDist;

    let bandwidth = graph.getTotalBandwidthUsage();
    let timestamps = new Array();
    let proto = new Array();
    let overlay = new Array();
    const options = {
        hour12 : false,
        hour:  "2-digit",
        minute: "2-digit",
    };

    let nodeCount = graph.getNodeCount();
    for (const [key, value] of [...bandwidth.entries()].sort()) {
        let date = (new Date(key / 1000 / 1000)).toLocaleTimeString("en-US",options);
        timestamps.push(date);
        proto.push(value.Protocol * conversionToKBPS / nodeCount); // bytes to Kbps
        overlay.push(value.Overlay * conversionToKBPS / nodeCount); // bytes to Kbps
    }

    let averageBWGraph = 'total-bandwidth';
    analyticsCharts[averageBWGraph].data.labels = timestamps;
    analyticsCharts[averageBWGraph].data.datasets[0].data = proto;
    analyticsCharts[averageBWGraph].data.datasets[1].data = overlay;

    if (currentAnalyticsChart.name === averageBWGraph || currentAnalyticsChart.name === distributionGraph) {
        updateAnalyticsSelection(currentAnalyticsChart.name);
    }
}

function resetReplayUI(element) {
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
    case "view-analytics":
        handleToolViewAnalytics(this);
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

export function SetPingToolState(enabled, active) {
    let subtool = document.getElementById("ping-start-stop");

    if (!enabled && subtool.className.includes("active")) {
        subtool.className = subtool.className.replace(" active", "");
        let tooltip = subtool.getElementsByClassName("tooltiptext")[0];
        tooltip.textContent = "Start Pings";

        if (subtool.className.includes("sub-active")) {
            subtool.className = subtool.className.replace(" sub-active", "");
        }
    } else if (enabled) {
        if (!subtool.className.includes("active")) {
            subtool.className += " active";
            let tooltip = subtool.getElementsByClassName("tooltiptext")[0];
            tooltip.textContent = "Stop Pings";
        }

        if (active && !subtool.className.includes("sub-active")) {
            subtool.className += " sub-active";
        } else if (!active && subtool.className.includes("sub-active")) {
            subtool.className = subtool.className.replace(" sub-active", "");
        }
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

function handleToolViewAnalytics(subtool) {
    setupBaseModal("analytics-modal");
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
        validSubcommands.set("DropRates", ["Overall", "Keepalive", "TreeAnnouncement", "VirtualSnakeBootstrap", "WakeupBroadcast", "OverlayTraffic"]);

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
        resetReplayUI(subtool);

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

function handleAnalyticsSelect() {
    let selection = document.getElementById("analyticsDropdown").value;
    document.getElementById("analyticsDropdown").blur();

    updateAnalyticsSelection(selection);
}
document.getElementById("analyticsDropdown").onchange = handleAnalyticsSelect;
document.getElementById("analyticsDropdown").value = 'routing-table-size';

function updateAnalyticsSelection(selection) {
    let chartParams = analyticsCharts[selection];
    currentAnalyticsChart.name = selection;
    currentAnalyticsChart.chart.destroy();
    currentAnalyticsChart.chart = new Chart(document.getElementById('networkAnalytics').getContext('2d'), {
        type: chartParams.type,
        data: chartParams.data,
        options: chartParams.options
    });
}


const bwTimestamps = new Array(11);

function assignTimestamps() {
    let minuteDelta = 1;
    let nowRoundedUp = new Date();
    let minutes = nowRoundedUp.getMinutes();
    let remainder = minuteDelta - (minutes % minuteDelta);
    nowRoundedUp.setMinutes(minutes + remainder, 0, 0);
    let newMinutes = nowRoundedUp.getMinutes();
    const options = {
        hour12 : false,
        hour:  "2-digit",
        minute: "2-digit",
    };

    for (let i = 0; i < 11; i++) {
        let newTime = structuredClone(nowRoundedUp);
        newTime.setMinutes(newMinutes - minuteDelta * i);
        bwTimestamps[10 - i] = newTime.toLocaleTimeString("en-US",options);
    }
}
assignTimestamps();

var analyticsCharts = {
    'routing-table-size': {
        type: 'bar',
        data: {
            datasets: [{
                label: '# of Nodes',
                data: [],
                backgroundColor: [
                    "rgba(126,105,255,0.5)"
                ],
                borderWidth: 0,
                barPercentage: 1,
                categoryPercentage: 1
            }]
        },
        options: {
            animation: {
                duration: 300,
            },
            interaction: {
                intersect: true,
                mode: 'index',
            },
            scales: {
                x: {
                    beginAtZero: true,
                    type: 'linear',
                    ticks: {
                        stepSize: 1
                    },
                    title: {
                        display: true,
                        text: '# of Routes',
                        font: {
                            size: 14
                        }
                    }
                },
                y: {
                    beginAtZero: true,
                    title: {
                        display: true,
                        text: '# of Nodes',
                        font: {
                            size: 14
                        }
                    }
                }
            },
            layout: {
                padding: {
                    top: 20
                }
            },
            plugins: {
                legend: {
                    display: false,
                },
                tooltip: {
                    callbacks: {
                        title: (items) => {
                            if (!items.length) {
                                return '';
                            }
                            const item = items[0];
                            const x = item.parsed.x;
                            return `Routes: ${x}`;
                        }
                    }
                }
            }
        }
    },
    'total-bandwidth': {
        type: 'line',
        data: {
            labels: bwTimestamps,
            datasets: [{
                label: "Protocol Traffic",
                fill: true,
                backgroundColor: "rgba(35,140,245,0.5)",
                borderColor: "rgba(35,140,245,0.8)",
                data: [],
                yAxisID: 'y'
            }, {
                label: "Overlay Traffic",
                fill: true,
                backgroundColor: "rgba(126,105,255,0.5)",
                borderColor: "rgba(126,105,255,0.8)",
                data: [],
                yAxisID: 'y2'
            }],
        },
        options: {
            animation: {
                duration: 300,
            },
            interaction: {
                intersect: false,
                mode: 'index',
            },
            responsive: true,
            scales: {
                x: {
                    title: {
                        display: true,
                        text: 'Timestamp',
                        font: {
                            size: 14
                        }
                    }
                },
                y: {
                    beginAtZero: true,
                    title: {
                        display: true,
                        text: 'Bandwidth (kb/s)',
                        font: {
                            size: 14
                        }
                    }
                },
                y2: {
                    position: 'right',
                    beginAtZero: true,
                    title: {
                        display: true,
                        text: 'Bandwidth (kb/s)',
                        font: {
                            size: 14
                        }
                    },
                    grid: {
                        drawOnChartArea: false // only want the grid lines for one axis to show up
                    }
                }
            },
            layout: {
                padding: {
                    top: 20
                }
            }
        }
    },
    'bandwidth-distribution': {
        type: 'bar',
        data: {
            labels: [],
            datasets: [{
                label: 'Protocol Traffic',
                data: [],
                backgroundColor: [
                    "rgba(126,105,255,0.5)"
                ],
                borderWidth: 0,
                barPercentage: 1,
                categoryPercentage: 1,
                yAxisID: 'y'
            }]
        },
        options: {
            animation: {
                duration: 300,
            },
            interaction: {
                intersect: false,
                mode: 'index',
            },
            scales: {
                x: {
                    type: 'logarithmic',
                    // type: 'linear',
                    title: {
                        display: true,
                        text: 'Bandwidth (kb/s)',
                        font: {
                            size: 14
                        }
                    }
                },
                y: {
                    // stacked: false,
                    beginAtZero: true,
                    title: {
                        display: true,
                        text: '# of Nodes',
                        font: {
                            size: 14
                        }
                    }
                }
            },
            layout: {
                padding: {
                    top: 20
                }
            },
            plugins: {
                legend: {
                    display: true,
                },
                tooltip: {
                    callbacks: {
                        title: (items) => {
                            if (!items.length) {
                                return '';
                            }
                            const item = items[0];
                            const x = item.label;
                            return `BW: < ${x} kb/s`;
                        }
                    }
                }
            }
        }
    }
};

let routeChartParams = analyticsCharts['routing-table-size'];
let currentAnalyticsChart = {
    name: 'routing-table-size',
    chart: new Chart(document.getElementById('networkAnalytics').getContext('2d'), {
    type: routeChartParams.type,
    data: routeChartParams.data,
    options: routeChartParams.options
    })
};
