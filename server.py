#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""web server for ImageWebPlayer."""
import logging
import mimetypes
import os
import re
import argparse
from functools import cmp_to_key
from io import BytesIO
from aiohttp import web

import coloredlogs
import libarchive.public


logger = logging.getLogger('ImageWebPlayer')
coloredlogs.install()

_filename_num_pattern = re.compile(r'^(?P<suffix>^.*?)(?P<sn>\d*)(?P<ext>\..*?)?$')


def filename_compare(x, y):
    gx = _filename_num_pattern.match(x)
    gy = _filename_num_pattern.match(y)
    if gx.group('suffix') == gy.group('suffix') and gx.group('ext') == gy.group('ext'):
        snx = gx.group('sn')
        sny = gy.group('sn')
        if snx and sny and int(snx) != int(sny):
            return int(snx) - int(sny)
    return (x > y) - (x < y)


class OpenedArchive:

    def __init__(self, filepath):
        self.filepath = filepath

    def close(self):
        pass
        # self.context.__close__()

    def listdir(self, path):
        path = path.rstrip('/')
        dirs = []
        files = []
        with libarchive.public.file_reader(self.filepath) as entries:
            for entry in entries:
                dir, name = os.path.split(entry.pathname.rstrip('/'))
                if dir == path:
                    if entry.filetype.IFDIR:
                        dirs.append(name)
                    else:
                        files.append(name)
        return dirs, files

    def readfile(self, path, writer=None):
        ret = BytesIO()
        with libarchive.public.file_reader(self.filepath) as entries:
            for entry in entries:
                if entry.pathname == path:
                    for block in entry.get_blocks():
                        ret.write(block)
        return ret


class MainHandler:

    def __init__(self, rootdir):
        self.rootdir = rootdir
        self.current_dir = os.path.dirname(os.path.abspath(__file__))
        self.archive = None
        logger.info("current_dir: {}".format(self.current_dir))

    async def index(self, request):
        return web.FileResponse(os.path.join(self.current_dir, 'index.html'))

    async def listdir(self, request):
        return self.list_impl(request)

    async def listimg(self, request):
        return self.list_impl(request, imgonly=True)

    def list_impl(self, request, imgonly=False):
        path_in_url = request.match_info['path']
        if path_in_url.find('//') != -1:
            dirs, files = self.list_archive(path_in_url)
        else:
            path = os.path.join(self.rootdir, path_in_url)
            logger.info('path: {}'.format(path))
            files = []
            dirs = []
            for name in os.listdir(path):
                if os.path.isfile(os.path.join(path, name)):
                    files.append(name)
                elif not imgonly:
                    dirs.append(name)

        def filter_func(name):
            ext = name.split('.')[-1]
            return ext in ['png', 'jpg', 'jpeg']

        if imgonly:
            files = filter(filter_func, files)

        files = sorted(files, key=cmp_to_key(filename_compare))
        # files = sorted(files)
        return web.json_response(dict(files=files, dirs=dirs))

    def get_archive(self, filepath):
        if self.archive is None or self.archive.filepath != filepath:
            if self.archive is not None:
                self.archive.close()
            self.archive = OpenedArchive(filepath)
        return self.archive

    def list_archive(self, path_in_url):
        print(path_in_url.split('//'))
        archive_file, path_in_archive = path_in_url.split('//')
        archive_file = os.path.join(self.rootdir, archive_file)
        archive_file = self.get_archive(archive_file)
        dirs, files = archive_file.listdir(path_in_archive)
        return dirs, files

    def archive_file(self, request):
        path_in_url = request.match_info['path']
        archive_file, path_in_archive = path_in_url.split('//')
        archive_file = os.path.join(self.rootdir, archive_file)
        archive_file = self.get_archive(archive_file)
        content = archive_file.readfile(path_in_archive)
        print(content)
        return web.Response(body=content.getvalue(),
                            content_type=mimetypes.guess_type(path_in_archive)[0])


def main():
    parser = argparse.ArgumentParser(description='ImageWebPlayer')
    parser.add_argument('root', type=str, help='rootdir to view')
    parser.add_argument('-p', '--port', type=str, default='8080', help='listening port')
    parser.add_argument('-b', '--bind', type=str, default='127.0.0.1', help='bind address')
    args = parser.parse_args()

    rootdir = os.path.abspath(args.root)

    logger.info("root dir: {}".format(rootdir))
    if not os.path.isdir(rootdir):
        logger.error("root dir is invalid.")
        return

    app = web.Application()
    main_handler = MainHandler(rootdir=rootdir)
    app.add_routes([web.get('/', main_handler.index)])
    app.add_routes([web.get('/list/{path:.*}', main_handler.listdir)])
    app.add_routes([web.get('/listimg/{path:.*}', main_handler.listimg)])
    app.add_routes([web.static('/files', rootdir)])
    app.add_routes([web.get('/archive_file/{path:.*}', main_handler.archive_file)])

    web.run_app(app, host=args.bind, port=args.port)


if __name__ == '__main__':
    main()
