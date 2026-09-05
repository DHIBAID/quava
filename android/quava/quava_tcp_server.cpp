#include "quava_tcp_server.h"

#include <QCryptographicHash>
#include <QDateTime>
#include <QDebug>
#include <QHostAddress>
#include <QHostInfo>
#include <QNetworkInterface>
#include <QRandomGenerator>
#include <QTcpSocket>

namespace {
QString makeDeviceName() {
	const QString hostName = QHostInfo::localHostName().trimmed();
	if (!hostName.isEmpty()) {
		return hostName;
	}
	return QStringLiteral("Quava Android");
}

QString makeDeviceId() {
	const QByteArray seed = makeDeviceName().toUtf8() + QByteArrayLiteral("/android");
	const QByteArray digest = QCryptographicHash::hash(seed, QCryptographicHash::Sha256).left(16);
	return QString::fromUtf8(digest.toHex());
}
}  // namespace

QuavaTcpServer::QuavaTcpServer(QObject* parent)
	: QObject(parent), localDeviceId_(makeDeviceId()), localDeviceName_(makeDeviceName()) {
	setStatus(QStringLiteral("Ready to advertise and accept pairing requests"));
	connect(&server_, &QTcpServer::newConnection, this, [this]() {
		if (pendingSocket_ != nullptr) {
			// only accept one pending pairing at a time
			QTcpSocket* s = server_.nextPendingConnection();
			s->disconnectFromHost();
			return;
		}
		QTcpSocket* sock = server_.nextPendingConnection();
		// take ownership reference; store pointer immediately
		pendingSocket_ = sock;
		qDebug() << "QuavaTcpServer: new connection from" << sock->peerAddress().toString() << sock->peerPort();
		const QString peer = QStringLiteral("%1:%2").arg(sock->peerAddress().toString(), QString::number(sock->peerPort()));
		setPendingPeerName(peer);
		// generate a random 6-digit code and present as "123 456"
		int code = QRandomGenerator::global()->bounded(1000000);
		QString formatted = QString::asprintf("%03d %03d", code / 1000, code % 1000);
		setPairingCode(formatted);
		setConfirmationPending(true);
		setStatus(QStringLiteral("Awaiting user confirmation for pairing"));

		// capture the concrete socket pointer so we can safely compare
		connect(sock, &QTcpSocket::disconnected, this, [this, sock]() {
			if (pendingSocket_ == sock) {
				// clear ownership first so other paths won't double-delete
				pendingSocket_ = nullptr;
				sock->deleteLater();
			}
			qDebug() << "QuavaTcpServer: socket disconnected" << sock->peerAddress().toString() << sock->peerPort() << "error:" << sock->errorString();
			setConfirmationPending(false);
			setPairingCode(QString());
			setPendingPeerName(QString());
			setStatus(QStringLiteral("Ready to advertise and accept pairing requests"));
		});
	});
}

bool QuavaTcpServer::start(quint16 port) {
	if (!server_.listen(QHostAddress::AnyIPv4, port)) {
		setStatus(QStringLiteral("TCP server failed to listen on %1").arg(port));
		return false;
	}
	setStatus(QStringLiteral("Listening on port %1, waiting for pairing").arg(server_.serverPort()));
	return true;
}

QString QuavaTcpServer::status() const {
	return status_;
}

bool QuavaTcpServer::paired() const {
	return paired_;
}

QString QuavaTcpServer::pairingCode() const {
	return pairingCode_;
}

QString QuavaTcpServer::pendingPeerName() const {
	return pendingPeerName_;
}

bool QuavaTcpServer::confirmationPending() const {
	return confirmationPending_;
}

QString QuavaTcpServer::localDeviceId() const {
	return localDeviceId_;
}

QString QuavaTcpServer::localDeviceName() const {
	return localDeviceName_;
}

void QuavaTcpServer::confirmPairing() {
	if (pendingSocket_) {
		// Proxy the in-progress pairing connection to a host-side Go responder
		// This allows testing the full crypto flow without embedding OpenSSL in the app.
		QTcpSocket* inbound = pendingSocket_;
		// clear ownership so disconnected handler won't double-delete
		pendingSocket_ = nullptr;

		QTcpSocket* outbound = new QTcpSocket(this);
		const QHostAddress hostAddr = QHostAddress::LocalHost;	// connects to 127.0.0.1 on device
		const quint16 hostPort = 48273;							// host responder port (use adb reverse tcp:48273:48273)

		// Forward bytes from inbound -> outbound
		connect(inbound, &QTcpSocket::readyRead, this, [inbound, outbound]() {
			const QByteArray data = inbound->readAll();
			if (!data.isEmpty()) {
				qDebug() << "QuavaTcpServer: inbound->outbound" << data.size() << "bytes";
				outbound->write(data);
			}
		});

		// Forward bytes from outbound -> inbound
		connect(outbound, &QTcpSocket::readyRead, this, [inbound, outbound]() {
			const QByteArray data = outbound->readAll();
			if (!data.isEmpty()) {
				qDebug() << "QuavaTcpServer: outbound->inbound" << data.size() << "bytes";
				inbound->write(data);
			}
		});

		// When either side disconnects, tear down both sockets
		auto teardown = [this, inbound, outbound]() {
			if (inbound->state() != QAbstractSocket::UnconnectedState) {
				inbound->disconnectFromHost();
			}
			if (outbound->state() != QAbstractSocket::UnconnectedState) {
				outbound->disconnectFromHost();
			}
			inbound->deleteLater();
			outbound->deleteLater();
			setConfirmationPending(false);
			setPairingCode(QString());
			setPendingPeerName(QString());
			setStatus(QStringLiteral("Ready to advertise and accept pairing requests"));
		};

		connect(inbound, &QTcpSocket::disconnected, this, [inbound, outbound, teardown]() {
			qDebug() << "QuavaTcpServer: inbound disconnected, tearing down";
			teardown();
		});
		connect(outbound, &QTcpSocket::disconnected, this, [inbound, outbound, teardown]() {
			qDebug() << "QuavaTcpServer: outbound disconnected, tearing down";
			teardown();
		});

		connect(outbound, &QTcpSocket::connected, this, [this]() {
			qDebug() << "QuavaTcpServer: outbound connected to host responder";
			setPaired(true);
			setStatus(QStringLiteral("Proxying pairing to host responder"));
		});

		connect(outbound, QOverload<QAbstractSocket::SocketError>::of(&QAbstractSocket::errorOccurred), this, [outbound](QAbstractSocket::SocketError err) {
			qDebug() << "QuavaTcpServer: outbound socket error" << err << outbound->errorString();
		});

		// attempt to connect to the host responder (use adb reverse to map device port to host)
		outbound->connectToHost(hostAddr, hostPort);
		setConfirmationPending(false);
		setPairingCode(QString());
		setPendingPeerName(QString());
	}
}

void QuavaTcpServer::rejectPairing() {
	if (pendingSocket_) {
		QTcpSocket* sock = pendingSocket_;
		pendingSocket_ = nullptr;
		sock->disconnectFromHost();
		sock->deleteLater();
	}
	setConfirmationPending(false);
	setPaired(false);
	setPairingCode(QString());
	setPendingPeerName(QString());
	setStatus(QStringLiteral("Pairing rejected"));
}

void QuavaTcpServer::setStatus(const QString& status) {
	if (status_ == status) {
		return;
	}
	status_ = status;
	emit statusChanged();
}

void QuavaTcpServer::setPaired(bool paired) {
	if (paired_ == paired) {
		return;
	}
	paired_ = paired;
	emit pairedChanged();
}

void QuavaTcpServer::setPairingCode(const QString& code) {
	if (pairingCode_ == code) {
		return;
	}
	pairingCode_ = code;
	emit pairingCodeChanged();
}

void QuavaTcpServer::setPendingPeerName(const QString& name) {
	if (pendingPeerName_ == name) {
		return;
	}
	pendingPeerName_ = name;
	emit pendingPeerNameChanged();
}

void QuavaTcpServer::setConfirmationPending(bool pending) {
	if (confirmationPending_ == pending) {
		return;
	}
	confirmationPending_ = pending;
	emit confirmationPendingChanged();
}
