FROM node:14.17
WORKDIR /workspace
COPY website-dev.sh /workspace

RUN git init kubevela.io

ENTRYPOINT [ "bash", "website-dev.sh" ]
CMD [ "-t", "start" ]