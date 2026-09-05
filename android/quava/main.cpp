#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>

#include "mdns_advertiser.h"
#include "quava_tcp_server.h"

int main(int argc, char* argv[]) {
	QGuiApplication app(argc, argv);

	QuavaMdnsAdvertiser advertiser;
	advertiser.start();

	QuavaTcpServer tcpServer;
	if (!tcpServer.start(48273)) {
		return -1;
	}

	QQmlApplicationEngine engine;
	engine.rootContext()->setContextProperty(QStringLiteral("quavaTcpServer"), &tcpServer);
	QObject::connect(
		&engine,
		&QQmlApplicationEngine::objectCreationFailed,
		&app,
		[]() { QCoreApplication::exit(-1); },
		Qt::QueuedConnection);
	engine.loadFromModule("quava", "Main");

	return QGuiApplication::exec();
}
