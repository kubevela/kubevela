FROM node:16.13.0
WORKDIR /workspace
COPY website-dev.sh /workspace

RUN git init kubevela.io

ENTRYPOINT [ "bash", "website-dev.sh" ]
CMD [ "-t", "start" ]