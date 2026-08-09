# The UI5 application has no build step: it is static files plus the UI5
# runtime, so the image is nginx with the sources and a proxy to the API.
FROM nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY index.html /usr/share/nginx/html/index.html
COPY webapp /usr/share/nginx/html/webapp

EXPOSE 8080
