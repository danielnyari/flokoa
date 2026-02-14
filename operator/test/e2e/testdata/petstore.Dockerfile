FROM swaggerapi/petstore:latest

# Make directories writable by non-root user (UID 1000)
RUN chown -R 1000:1000 /petstore /var/log

USER 1000
