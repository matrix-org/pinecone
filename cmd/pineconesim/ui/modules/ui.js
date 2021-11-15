import { graph } from "../main.js";

let leftShown = true;
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
    }
}

function setupNetworkSelection() {
    let selectionTabs = document.getElementsByClassName("netselect");
    for (let i = 0; i < selectionTabs.length; i++) {
        selectionTabs[i].onclick = selectNetworkType;
    }
}
setupNetworkSelection();


function selectNavTab() {
    let selectionTabs = document.getElementsByClassName("navselect");
    for (let i = 0; i < selectionTabs.length; i++) {
        selectionTabs[i].className = selectionTabs[i].className.replace(" active", "");
    }

    this.className += " active";
}

function setupTopnavSelection() {
    let selectionTabs = document.getElementsByClassName("navselect");
    for (let i = 0; i < selectionTabs.length; i++) {
        selectionTabs[i].onclick = selectNavTab;
    }
}
setupTopnavSelection();
