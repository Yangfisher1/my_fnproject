FROM alpine:latest

# Install hotwrap binary in your container
COPY --from=fnproject/hotwrap:latest  /hotwrap /hotwrap

CMD "/bin/rev"

ENTRYPOINT ["/hotwrap"]
