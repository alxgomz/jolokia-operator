FROM openjdk:26-ea-21-slim

ADD --chmod=644 https://repo1.maven.org/maven2/org/jolokia/jolokia-agent-jvm/2.5.1/jolokia-agent-jvm-2.5.1-javaagent.jar /jolokia-agent.jar

COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
