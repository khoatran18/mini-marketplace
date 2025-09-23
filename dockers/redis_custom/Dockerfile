FROM redis:8.2.1-alpine

COPY create_cluster.sh /usr/local/bin/create_cluster.sh

RUN chmod +x /usr/local/bin/create_cluster.sh

#RUN dos2unix /usr/local/bin/create_cluster.sh && chmod +x /usr/local/bin/create_cluster.sh

ENTRYPOINT ["/usr/local/bin/create_cluster.sh"]