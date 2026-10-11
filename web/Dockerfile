FROM nginx:1.27-alpine

# API_URL can be given at build time (--build-arg) or at run time (env var).
ARG API_URL=""
ENV PORT=8080 \
    API_URL=${API_URL}

COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY 40-sortly-config.sh /docker-entrypoint.d/40-sortly-config.sh
COPY index.html style.css app.js config.js /usr/share/nginx/html/

EXPOSE 8080
