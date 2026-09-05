#include "mdns_advertiser.h"

#include <QNetworkInterface>
#include <QStringList>

namespace {
QString sanitizeHostName(QString name) {
	name = name.trimmed().toLower();
	if (name.isEmpty()) {
		name = QStringLiteral("android.local.");
	}

	if (!name.endsWith(QLatin1Char('.'))) {
		name.append(QLatin1Char('.'));
	}

	return name;
}

QString sanitizeInstanceName(QString name) {
	name = name.trimmed();
	if (name.isEmpty()) {
		name = QStringLiteral("Android");
	}
	return name;
}
}  // namespace

QuavaMdnsAdvertiser::QuavaMdnsAdvertiser(QObject* parent)
	: QObject(parent) {
	connect(&timer_, &QTimer::timeout, this, [this]() { sendPacket(); });
	timer_.setInterval(2000);
}

void QuavaMdnsAdvertiser::start() {
	const QString instanceName = QStringLiteral("Quava Android");
	const QString hostName = QStringLiteral("quava-android.local.");
	const QHostAddress address = localIPv4Address();
	packet_ = buildPacket(instanceName, hostName, address, 48273);
	sendPacket();
	timer_.start();
}

void QuavaMdnsAdvertiser::sendPacket() {
	if (packet_.isEmpty()) {
		return;
	}

	const qint64 written = socket_.writeDatagram(packet_, multicastAddress_, multicastPort_);
	if (written < 0) {
		qWarning("Quava mDNS advertisement failed: %s", qPrintable(socket_.errorString()));
	}
}

QByteArray QuavaMdnsAdvertiser::encodeDnsName(const QString& name) {
	QString trimmed = name.trimmed();
	if (trimmed == QLatin1String(".")) {
		return QByteArray(1, char(0));
	}

	if (trimmed.endsWith(QLatin1Char('.'))) {
		trimmed.chop(1);
	}

	QByteArray encoded;
	const QStringList labels = trimmed.split(QLatin1Char('.'), Qt::SkipEmptyParts);
	for (const QString& label : labels) {
		const QByteArray utf8 = label.toUtf8();
		if (utf8.size() > 63) {
			continue;
		}
		encoded.append(char(utf8.size()));
		encoded.append(utf8);
	}
	encoded.append(char(0));
	return encoded;
}

QHostAddress QuavaMdnsAdvertiser::localIPv4Address() {
	const QList<QHostAddress> addresses = QNetworkInterface::allAddresses();
	for (const QHostAddress& address : addresses) {
		if (address.protocol() == QAbstractSocket::IPv4Protocol && !address.isLoopback()) {
			return address;
		}
	}

	return QHostAddress(QStringLiteral("127.0.0.1"));
}

QByteArray QuavaMdnsAdvertiser::buildPacket(const QString& instanceName, const QString& hostName, const QHostAddress& address, quint16 port) {
	const QString serviceName = QStringLiteral("_quava._udp.local.");
	const QString instanceFqdn = instanceName + QLatin1Char('.') + serviceName;
	const QByteArray serviceNameData = encodeDnsName(serviceName);
	const QByteArray ptrPayload = encodeDnsName(instanceFqdn);

	QByteArray ptrRecord = serviceNameData;
	ptrRecord.append(10, char(0));
	const int ptrOffset = serviceNameData.size();
	ptrRecord[ptrOffset + 1] = char(12);
	ptrRecord[ptrOffset + 3] = char(1);
	ptrRecord[ptrOffset + 7] = char(120);
	ptrRecord[ptrOffset + 9] = char(ptrPayload.size());
	ptrRecord.append(ptrPayload);

	const QByteArray instanceNameData = encodeDnsName(instanceFqdn);
	const QByteArray hostNameData = encodeDnsName(hostName);

	QByteArray srvPayload;
	srvPayload.append(char(0));
	srvPayload.append(char(0));
	srvPayload.append(char(0));
	srvPayload.append(char(0));
	srvPayload.append(char((port >> 8) & 0xff));
	srvPayload.append(char(port & 0xff));
	srvPayload.append(hostNameData);

	QByteArray srvRecord = instanceNameData;
	srvRecord.append(10, char(0));
	const int srvOffset = instanceNameData.size();
	srvRecord[srvOffset + 1] = char(33);
	srvRecord[srvOffset + 3] = char(1);
	srvRecord[srvOffset + 7] = char(120);
	srvRecord[srvOffset + 9] = char(srvPayload.size());
	srvRecord.append(srvPayload);

	QByteArray txtRecord = instanceNameData;
	txtRecord.append(10, char(0));
	const int txtOffset = instanceNameData.size();
	txtRecord[txtOffset + 1] = char(16);
	txtRecord[txtOffset + 3] = char(1);
	txtRecord[txtOffset + 7] = char(120);
	txtRecord[txtOffset + 9] = char(0);

	QByteArray aPayload;
	const QStringList octets = address.toString().split(QLatin1Char('.'));
	if (octets.size() == 4) {
		for (const QString& octet : octets) {
			aPayload.append(char(octet.toInt()));
		}
	} else {
		aPayload.append(char(127));
		aPayload.append(char(0));
		aPayload.append(char(0));
		aPayload.append(char(1));
	}

	QByteArray aRecord = encodeDnsName(hostName);
	aRecord.append(10, char(0));
	const int aOffset = hostNameData.size();
	aRecord[aOffset + 1] = char(1);
	aRecord[aOffset + 3] = char(1);
	aRecord[aOffset + 7] = char(120);
	aRecord[aOffset + 9] = char(aPayload.size());
	aRecord.append(aPayload);

	QByteArray packet;
	packet.append(char(0xbe));
	packet.append(char(0xef));
	packet.append(char(0x84));
	packet.append(char(0x00));
	packet.append(char(0x00));
	packet.append(char(0x00));
	packet.append(char(0x00));
	packet.append(char(0x04));
	packet.append(char(0x00));
	packet.append(char(0x00));
	packet.append(char(0x00));
	packet.append(char(0x00));
	packet.append(ptrRecord);
	packet.append(srvRecord);
	packet.append(txtRecord);
	packet.append(aRecord);
	return packet;
}