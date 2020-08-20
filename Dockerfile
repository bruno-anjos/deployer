FROM busybox
COPY ./deployer /deployer
COPY ./alternatives.txt /alternatives.txt

EXPOSE 50002

CMD ["/deployer", "-d"]