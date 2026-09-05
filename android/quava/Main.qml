import QtQuick
import QtQuick.Layouts
import QtQuick.Controls

ApplicationWindow {
    id: window
    width: 640
    height: 480
    minimumWidth: 200
    minimumHeight: 250
    visible: true
    title: qsTr("Quava Pairing Responder")

    Rectangle {
        anchors.fill: parent
        gradient: Gradient {
            GradientStop {
                position: 0.0
                color: "#07111f"
            }
            GradientStop {
                position: 0.55
                color: "#0b1b2d"
            }
            GradientStop {
                position: 1.0
                color: "#111827"
            }
        }

        ColumnLayout {
            anchors.centerIn: parent
            width: Math.min(parent.width - 32, 520)
            spacing: 18

            Label {
                text: qsTr("Quava")
                color: "#f8fafc"
                font.pixelSize: 44
                font.bold: true
                horizontalAlignment: Text.AlignHCenter
                Layout.fillWidth: true
            }

            Label {
                text: qsTr("Advertising and waiting for authenticated pairing requests on the local network.")
                color: "#cbd5e1"
                wrapMode: Text.WordWrap
                horizontalAlignment: Text.AlignHCenter
                Layout.fillWidth: true
            }

            Rectangle {
                Layout.fillWidth: true
                radius: 24
                color: "#0f172a"
                border.color: "#22304a"
                border.width: 1
                implicitHeight: 182

                ColumnLayout {
                    anchors.fill: parent
                    anchors.margins: 20
                    spacing: 10

                    Label {
                        text: qsTr("Device")
                        color: "#93c5fd"
                        font.pixelSize: 12
                        font.capitalization: Font.AllUppercase
                    }

                    Label {
                        text: quavaTcpServer.localDeviceName + " • " + quavaTcpServer.localDeviceId
                        color: "#f8fafc"
                        wrapMode: Text.WordWrap
                        Layout.fillWidth: true
                    }

                    Label {
                        text: qsTr("Status")
                        color: "#93c5fd"
                        font.pixelSize: 12
                        font.capitalization: Font.AllUppercase
                    }

                    Label {
                        text: quavaTcpServer.status
                        color: "#e2e8f0"
                        wrapMode: Text.WordWrap
                        Layout.fillWidth: true
                    }
                }
            }

            Rectangle {
                Layout.fillWidth: true
                radius: 20
                color: "#111c2c"
                border.color: "#314157"
                border.width: 1
                implicitHeight: 220

                ColumnLayout {
                    anchors.fill: parent
                    anchors.margins: 16
                    spacing: 12

                    Label {
                        text: qsTr("Pairing")
                        color: "#94a3b8"
                        font.pixelSize: 12
                        font.capitalization: Font.AllUppercase
                    }

                    Label {
                        text: quavaTcpServer.confirmationPending ? qsTr("Confirm the code below with the initiator.") : qsTr("Waiting for a pairing request.")
                        color: "#e2e8f0"
                        wrapMode: Text.WordWrap
                        Layout.fillWidth: true
                    }

                    Rectangle {
                        Layout.fillWidth: true
                        radius: 18
                        color: "#0f172a"
                        border.color: quavaTcpServer.confirmationPending ? "#38bdf8" : "#334155"
                        border.width: 1
                        implicitHeight: 88

                        ColumnLayout {
                            anchors.fill: parent
                            anchors.margins: 14
                            spacing: 4

                            Label {
                                text: qsTr("Pairing code")
                                color: "#94a3b8"
                                font.pixelSize: 11
                                font.capitalization: Font.AllUppercase
                            }

                            Label {
                                text: quavaTcpServer.pairingCode.length > 0 ? quavaTcpServer.pairingCode : qsTr("--- ---")
                                color: quavaTcpServer.confirmationPending ? "#f8fafc" : "#64748b"
                                font.pixelSize: 28
                                font.bold: true
                                Layout.fillWidth: true
                            }

                            Label {
                                text: quavaTcpServer.pendingPeerName.length > 0 ? qsTr("Initiator: %1").arg(quavaTcpServer.pendingPeerName) : qsTr("No active pairing")
                                color: "#cbd5e1"
                                Layout.fillWidth: true
                                wrapMode: Text.WordWrap
                            }
                        }
                    }

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: 10

                        Button {
                            text: qsTr("Reject")
                            enabled: quavaTcpServer.confirmationPending
                            Layout.fillWidth: true
                            onClicked: quavaTcpServer.rejectPairing()
                        }

                        Button {
                            text: qsTr("Confirm")
                            enabled: quavaTcpServer.confirmationPending
                            Layout.fillWidth: true
                            onClicked: quavaTcpServer.confirmPairing()
                        }
                    }
                }
            }
        }
    }
}
