function refreshContent(url) {
    htmx.ajax("GET", url, {target: "#content", select: "#content"})
}

function copySQLToClipboard() {
    // Get the textarea element
    const textarea = document.getElementById("sql-query");

    // Select the text
    textarea.select();
    textarea.setSelectionRange(0, 99999); // For mobile devices

    // Copy the text inside the textarea
    navigator.clipboard.writeText(textarea.value)
        .then(() => {
            document.getElementById('copy-sql').innerText = 'Copied!'
            setTimeout(function () {
                document.getElementById('copy-sql').innerText = 'Copy SQL Query'
            }, 1000)
        })
        .catch(err => {
            document.getElementById('copy-sql').innerText = 'Failed to copy: ' + err
        });
}

function toggleSQLQuery() {
    const sqlQuerySection = document.getElementById('sql-query-section')
    sqlQuerySection.classList.toggle('visually-hidden')
    // switch button text
    const toggleSQLQueryBtn = document.getElementById('toggle-sql-query-btn')
    if (sqlQuerySection.classList.contains('visually-hidden')) {
        toggleSQLQueryBtn.innerText = 'Show SQL Query'
    } else {
        toggleSQLQueryBtn.innerText = 'Hide SQL Query'
    }
}

function addLabel() {
    let label = document.getElementById('add-label-name').value;
    if (!label) {
        return
    }
    let url = new URL(window.location.href);
    url.searchParams.append("col", "label_" + label)
    refreshContent(url.toString())
}

function iso8601ToDatetimeLocal(isoStr) {
    const date = new Date(isoStr);
    return dateToDatetimeLocal(date)
}

function dateToDatetimeLocal(date) {
    const offset = date.getTimezoneOffset() * 60000; // Convert offset to milliseconds
    const localISOTime = (new Date(date.getTime() - offset)).toISOString().slice(0, -1);
    return localISOTime.substring(0, localISOTime.lastIndexOf(':'));
}

function datetimeLocalToIso8601(datetimeLocalStr) {
    return new Date(datetimeLocalStr).toISOString();
}

function updateStartEnd(event) {
    let url = new URL(window.location.href);
    const start = datetimeLocalToIso8601(document.getElementById('start-local').value);
    const end = datetimeLocalToIso8601(document.getElementById('end-local').value);
    url.searchParams.set('start', start);
    url.searchParams.set('end', end);
    url.searchParams.delete('range');
    refreshContent(url.toString())
}

function appendAlert(message) {
    const alertPlaceholder = document.getElementById('alert-placeholder')
    const wrapper = document.createElement('div')
    wrapper.innerHTML = [
        `<div class="alert alert-danger alert-dismissible" role="alert">`,
        `   <div>${message}</div>`,
        '   <button type="button" class="btn-close" data-bs-dismiss="alert" aria-label="Close"></button>',
        '</div>'
    ].join('')
    alertPlaceholder.append(wrapper)
}

htmx.onLoad(function (document) {
    document.querySelectorAll('.local-iso').forEach(function (input) {
        input.value = iso8601ToDatetimeLocal(input.getAttribute('iso-date'));
        input.addEventListener('change', updateStartEnd);
    });
})

document.body.addEventListener("htmx:responseError", function (event) {
    appendAlert(event.detail.xhr.responseText)
})
