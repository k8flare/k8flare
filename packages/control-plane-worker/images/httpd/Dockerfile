FROM busybox:1.37
RUN mkdir -p /www && printf 'ok\n' > /www/index.html
EXPOSE 8080
ENTRYPOINT ["httpd"]
CMD ["-f", "-p", "8080", "-h", "/www"]
