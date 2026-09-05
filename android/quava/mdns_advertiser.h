#pragma once

#include <QByteArray>
#include <QHostAddress>
#include <QList>
#include <QObject>
#include <QTimer>
#include <QUdpSocket>

class QuavaMdnsAdvertiser : public QObject {
   public:
	explicit QuavaMdnsAdvertiser(QObject* parent = nullptr);

	void start();

   private:
	static QByteArray encodeDnsName(const QString& name);
	static QHostAddress localIPv4Address();
	static QByteArray buildPacket(const QString& instanceName, const QString& hostName, const QHostAddress& address, quint16 port);
	void sendPacket();

	QUdpSocket socket_;
	QHostAddress multicastAddress_{QHostAddress(QStringLiteral("224.0.0.251"))};
	quint16 multicastPort_{5353};
	QTimer timer_;
	QByteArray packet_;
};