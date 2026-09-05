#pragma once

#include <QObject>
#include <QString>
#include <QTcpServer>
#include <QTcpSocket>

class QuavaTcpServer : public QObject {
	Q_OBJECT
	Q_PROPERTY(QString status READ status NOTIFY statusChanged)
	Q_PROPERTY(bool paired READ paired NOTIFY pairedChanged)
	Q_PROPERTY(QString pairingCode READ pairingCode NOTIFY pairingCodeChanged)
	Q_PROPERTY(QString pendingPeerName READ pendingPeerName NOTIFY pendingPeerNameChanged)
	Q_PROPERTY(bool confirmationPending READ confirmationPending NOTIFY confirmationPendingChanged)
	Q_PROPERTY(QString localDeviceId READ localDeviceId CONSTANT)
	Q_PROPERTY(QString localDeviceName READ localDeviceName CONSTANT)

   public:
	explicit QuavaTcpServer(QObject* parent = nullptr);

	bool start(quint16 port);
	QString status() const;
	bool paired() const;
	QString pairingCode() const;
	QString pendingPeerName() const;
	bool confirmationPending() const;
	QString localDeviceId() const;
	QString localDeviceName() const;
	Q_INVOKABLE void confirmPairing();
	Q_INVOKABLE void rejectPairing();

   signals:
	void statusChanged();
	void pairedChanged();
	void pairingCodeChanged();
	void pendingPeerNameChanged();
	void confirmationPendingChanged();

   private:
	void setStatus(const QString& status);
	void setPaired(bool paired);
	void setPairingCode(const QString& code);
	void setPendingPeerName(const QString& name);
	void setConfirmationPending(bool pending);

	// network
	QTcpSocket* pendingSocket_ = nullptr;

	QTcpServer server_;
	QString status_;
	QString pairingCode_;
	QString pendingPeerName_;
	QString localDeviceId_;
	QString localDeviceName_;
	bool confirmationPending_ = false;
	bool paired_ = false;
};
