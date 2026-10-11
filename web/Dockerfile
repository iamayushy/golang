FROM nginx:1.27-alpine

ENV PORT=8080 \
    API_URL=http://localhost:8080

COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY 40-sortly-config.sh /docker-entrypoint.d/40-sortly-config.sh
COPY index.html style.css app.js config.js /usr/share/nginx/html/

EXPOSE 8080
